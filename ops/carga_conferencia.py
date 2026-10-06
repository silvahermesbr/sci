#!/usr/bin/env python3
"""P5 (ordem 06/10, item 6) — CARGA DE CONFERÊNCIA (instância efêmera :14011).
Adaptado de qa/cia_0410/seed_cia.py (mesma Sess por persona via urllib+cookies).

Cenário de instabilidade relatado: sistema "caiu" após adicionar algumas
conferências. Carga: 1 grupo c/ 4 setores e ~100 pessoas; 1 conferência
base + criação de muitas conferências; 10-20 clientes de pooling (tick 2s,
GET /api/conferencia/estado — backend P4; com SCI_POOL_HOJE=1 usa /hoje,
medindo o ANTES); rajadas de POST marcar por fases.

Métricas por fase: latências individuais → p50/p95/max, taxa de erro,
elapsed. Uso:
  python3 ops/carga_conferencia.py [--clientes 12] [--fase-segundos 25]
Saída: resumo por fase no stdout (o relatório da frente consolida).

Knobs: SCI_PORT=14011 · SCI_DATA_DIR=/tmp/sci_carga (o script NÃO sobe o
servidor — subir antes:
  SCI_DATA_DIR=/tmp/sci_carga SCI_PORT=14011 ./sci_ci &
e matar pelo PID gravado; checar órfãos com pgrep -f sci_ci antes).
"""
import json, os, sys, time, threading, http.cookiejar, urllib.request, urllib.error, statistics

BASE = os.environ.get('SCI_BASE', 'http://127.0.0.1:14011')
N_CLIENTES = int(os.environ.get('SCI_CLIENTES', '12'))
FASE_SEG = float(os.environ.get('SCI_FASE_SEG', '25'))
POOL_HOJE = os.environ.get('SCI_POOL_HOJE', '') == '1'  # 1 = medir o ANTES (pooling /hoje)
SENHA = 'Carga@2026'


class Sess:
    def __init__(s):
        s.jar = http.cookiejar.CookieJar()
        s.op = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(s.jar))

    def req(s, method, path, body=None, timeout=30):
        data = json.dumps(body).encode() if body is not None else None
        r = urllib.request.Request(BASE + path, data=data, method=method)
        r.add_header('Content-Type', 'application/json')
        r.add_header('X-SCI', '1')
        t0 = time.perf_counter()
        try:
            with s.op.open(r, timeout=timeout) as resp:
                resp.read()
                return resp.status, (time.perf_counter() - t0) * 1000.0
        except urllib.error.HTTPError as e:
            try:
                e.read()
            except Exception:
                pass
            return e.code, (time.perf_counter() - t0) * 1000.0
        except Exception:
            return 0, (time.perf_counter() - t0) * 1000.0

    def req_json(s, method, path, body=None, timeout=30):
        """Setup: igual a req(), mas devolve (status, corpo JSON) — sem métrica."""
        data = json.dumps(body).encode() if body is not None else None
        r = urllib.request.Request(BASE + path, data=data, method=method)
        r.add_header('Content-Type', 'application/json')
        r.add_header('X-SCI', '1')
        try:
            with s.op.open(r, timeout=timeout) as resp:
                return resp.status, json.loads(resp.read().decode() or '{}')
        except urllib.error.HTTPError as e:
            try:
                return e.code, json.loads(e.read().decode() or '{}')
            except Exception:
                return e.code, {}
        except Exception:
            return 0, {}


def percentile(vals, p):
    if not vals:
        return 0.0
    vs = sorted(vals)
    k = max(0, min(len(vs) - 1, int(round((p / 100.0) * len(vs) + 0.5)) - 1))
    return vs[k]


def resumo(nome, lats, erros, elapsed):
    ok = [l for l in lats if l is not None]
    total = len(lats)
    return {
        'fase': nome, 'requisicoes': total, 'erros': erros,
        'erro_pct': round(100.0 * erros / total, 2) if total else 0.0,
        'p50_ms': round(percentile(ok, 50), 1), 'p95_ms': round(percentile(ok, 95), 1),
        'max_ms': round(max(ok), 1) if ok else 0.0,
        'rps': round(total / elapsed, 2) if elapsed > 0 else 0.0,
    }


RESULTADOS = []
LOCK = threading.Lock()


class Cliente(threading.Thread):
    """Pooling tick 2s (/estado por default, /hoje com SCI_POOL_HOJE=1) + rajadas."""

    def __init__(s, idx, sessao, estado):
        super().__init__(daemon=True)
        s.idx, s.s, s.st = idx, sessao, estado
        s.parar = threading.Event()

    def run(s):
        prox_tick = time.monotonic()
        prox_rajada = time.monotonic() + 3.0 + s.idx * 0.7
        while not s.parar.is_set():
            agora = time.monotonic()
            if agora >= prox_tick:
                prox_tick = agora + 2.0
                st, ms = s.s.req('GET', '/api/conferencia/estado' if not POOL_HOJE else '/api/conferencia/hoje')
                with LOCK:
                    s.st['lats_pool'].append(ms if st == 200 else None)
                    s.st['erros_pool'] += 0 if st in (200, 401) else 1
            if agora >= prox_rajada:
                prox_rajada = agora + 6.0 + (s.idx % 5)
                s.rajada()
            time.sleep(0.05)

    def rajada(s):
        with LOCK:
            conf = s.st.get('conf_id')
            pessoas = list(s.st['pessoas'])[:40]
            i = s.st['cursor'] % max(1, len(pessoas))
            s.st['cursor'] += 1
        if not conf or not pessoas:
            return
        for k in range(10):
            p = pessoas[(i + k) % len(pessoas)]
            st, ms = s.s.req('POST', '/api/conferencia/marcar', {
                'conferencia_id': conf, 'pessoa_id': p,
                'situacao': 'presente' if k % 2 else 'falta', 'verificado': 1})
            with LOCK:
                s.st['lats_marca'].append(ms if st == 200 else None)
                s.st['erros_marca'] += 0 if st in (200, 400, 403, 404) else 1


def main():
    for pg in os.popen('pgrep -f sci_ci').read().split():
        print(f'AVISO: processo sci_ci ativo (pid {pg}) — confirme que é o da carga (/tmp/sci_carga).')

    adm = Sess()
    st, _ = adm.req_json('POST', '/api/login', {'login': 'admin', 'senha': 'admin'})
    assert st == 200, f'login admin: {st} (suba o servidor: SCI_DATA_DIR=/tmp/sci_carga SCI_PORT=14011 ./sci_ci &)'
    st, b = adm.req_json('POST', '/api/grupos', {'nome': 'Cia Carga', 'login': 'ger_carga', 'senha': SENHA, 'nome_guerra': 'TEN CARGA'})
    if st != 200:
        # idempotente: seed repetido na MESMA instância (rodadas ANTES/DEPOIS)
        assert 'UNIQUE constraint failed: grupos.nome' in str(b.get('erro', '')), f'grupo: {st} {b}'
        print('seed: grupo Cia Carga já existe (instância reaproveitada)')

    ger = Sess()
    st, _ = ger.req_json('POST', '/api/login', {'login': 'ger_carga', 'senha': SENHA})
    assert st == 200, 'login gerente'

    # 4 setores × ~100 pessoas
    setores = []
    for i in range(4):
        st, b = ger.req_json('POST', '/api/catalogo/setores', {'nome': f'Setor Carga {i + 1}', 'sigla': f'SC{i + 1}'})
        if st == 200:
            setores.append(int(b['id']))
        else:
            # idempotente: mesma instância — procura o setor no catálogo
            st2, b2 = ger.req_json('GET', '/api/catalogo/setores')
            assert st2 == 200, f'catalogo setores: {st2} {b2}'
            lista_s = b2.get('setores', b2) if isinstance(b2, dict) else b2
            setores.append(int(next(x['id'] for x in lista_s if x.get('nome') == f'Setor Carga {i + 1}')))
    pessoas = []
    st, b = ger.req_json('GET', '/api/pessoas')
    assert st == 200, f'pessoas: {st} {b}'
    lista_pessoas = b.get('pessoas', b) if isinstance(b, dict) else b
    existentes = {x['nome_guerra']: int(x['id']) for x in lista_pessoas if x.get('status', 'ativo') == 'ativo'}
    for n in range(100):
        guerra = f'MIL {n:03d}'
        if guerra in existentes:
            pessoas.append(existentes[guerra])
            continue
        sid = setores[n % 4]
        st, b = ger.req_json('POST', '/api/pessoas', {'nome_guerra': guerra, 'nome_completo': f'Militar Carga {n:03d}', 'setor_id': sid})
        if st == 200:
            pessoas.append(int(b.get('id') or b.get('pessoa_id')))
    print(f'seed: {len(setores)} setores, {len(pessoas)} pessoas')

    # conferência base (o objeto do pooling e das rajadas) — a MAIS RECENTE aberta
    st, b = ger.req_json('GET', '/api/conferencia/hoje')
    assert st == 200, f'hoje: {st} {b}'
    if b.get('conferencia') and b['conferencia'].get('id'):
        conf_base = int(b['conferencia']['id'])
        print(f'seed: conferência base reaproveitada #{conf_base}')
    else:
        st, b = ger.req_json('POST', '/api/conferencia/iniciar', {'nome': 'Conf Base Carga', 'local': 'Térreo'})
        assert st == 200, f'iniciar: {st} {b}'
        conf_base = int(b['id'])

    st_state = {'conf_id': conf_base, 'pessoas': pessoas, 'cursor': 0,
                'lats_pool': [], 'erros_pool': 0, 'lats_marca': [], 'erros_marca': 0}

    clientes = [Cliente(i, Sess(), st_state) for i in range(N_CLIENTES)]
    for c in clientes:
        c.s.req_json('POST', '/api/login', {'login': 'ger_carga', 'senha': SENHA})
        c.start()

    def rodar_fase(nome, seg, criador=None):
        l0, e0, m0, em0 = (list(st_state['lats_pool']), st_state['erros_pool'],
                           list(st_state['lats_marca']), st_state['erros_marca'])
        t0 = time.monotonic()
        fim = t0 + seg
        criadas = 0
        while time.monotonic() < fim:
            if criador:
                if criador() == 200:
                    criadas += 1
            time.sleep(0.2)
        el = time.monotonic() - t0
        with LOCK:
            lp = st_state['lats_pool'][len(l0):]
            ep = st_state['erros_pool'] - e0
            lm = st_state['lats_marca'][len(m0):]
            em = st_state['erros_marca'] - em0
        r = resumo(nome + ' [pool 2s]', lp, ep, el)
        RESULTADOS.append(r)
        print(json.dumps(r, ensure_ascii=False))
        if criador:
            r2 = resumo(f'{nome} [criar conferência ×{criadas}]', [], 0, el)
            r2['criacoes_ok'] = criadas
            print(json.dumps(r2, ensure_ascii=False))
        return criadas

    try:
        # FASE 1 — só pooling sobre a conferência base
        rodar_fase('F1 só-pooling', FASE_SEG)
        # FASE 2 — pooling + rajadas de marcar
        rodar_fase('F2 pooling+rajadas', FASE_SEG)
        # FASE 3 — pooling + CRIAÇÃO de muitas conferências (relato do Diretor)
        conf_extra = []
        def criar():
            st, b = ger.req_json('POST', '/api/conferencia/iniciar', {'nome': f'Conf Carga {time.time()}'})

            if st == 200:
                conf_extra.append(int(b['id']))
            return st
        rodar_fase('F3 criacao-conf', FASE_SEG, criador=criar)
        # FASE 4 — pico: rajadas + criação + pooling juntos
        def criar4():
            st, b = ger.req_json('POST', '/api/conferencia/iniciar', {'nome': f'Conf Pico {time.time()}'})
            if st == 200:
                conf_extra.append(int(b['id']))
            return st
        rodar_fase('F4 pico-tudo', FASE_SEG, criador=criar4)
    finally:
        for c in clientes:
            c.parar.set()
        for c in clientes:
            c.join(timeout=5)

    print('\n=== RESUMO ===')
    for r in RESULTADOS:
        print(json.dumps(r, ensure_ascii=False))
    print(f'conferências criadas na carga: {len(conf_extra) + 1} (base + extras)')
    if POOL_HOJE:
        print('modo: SCI_POOL_HOJE=1 — pooling pelo endpoint PESADO /hoje (medição do ANTES do P4)')


if __name__ == '__main__':
    main()
