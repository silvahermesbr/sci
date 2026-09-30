package main

// Autenticação: argon2id, sessões em banco (sobrevivem a restart),
// rate-limit de login por falhas, guarda de método inseguro (CSRF leve).

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	cookieSessao = "sci_sessao"
	ttlSessao    = 12 * time.Hour
)

type Usuario struct {
	ID           int64  `json:"id"`
	Login        string `json:"login"`
	Papel        string `json:"papel"` // admin | gerente | operador
	PessoaID     *int64 `json:"pessoa_id"`
	GrupoID      *int64 `json:"grupo_id"`
	NomeGuerra   string `json:"nome_guerra"`
	NomeCompleto string `json:"nome_completo"`
	SetorID      *int64 `json:"setor_id"`
	FuncaoID     *int64 `json:"funcao_id"`
}

type ctxKeyChave int

const ctxUsuario ctxKeyChave = 1

// ---------- senha (argon2id, formato PHC) ----------

func hashSenha(senha string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	// OWASP-recomendado p/ footprint mínimo: m=19 MiB, t=2, p=1
	key := argon2.IDKey([]byte(senha), salt, 2, 19*1024, 1, 32)
	return fmt.Sprintf("$argon2id$v=19$m=19456,t=2,p=1$%s$%s",
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

func verificaSenha(senha, phc string) bool {
	p := strings.Split(phc, "$")
	if len(p) != 6 || p[1] != "argon2id" {
		return false
	}
	var m, t, pp int
	if _, err := fmt.Sscanf(p[3], "m=%d,t=%d,p=%d", &m, &t, &pp); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(p[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(p[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(senha), salt, uint32(t), uint32(m), uint8(pp), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// ---------- rate limit por falhas (transiente em memória — não é dado de missão) ----------

type Limiter struct {
	mu     sync.Mutex
	falhas map[string][]time.Time
}

func NovoLimiter() *Limiter {
	l := &Limiter{falhas: map[string][]time.Time{}}
	go func() {
		for range time.Tick(time.Hour) {
			l.mu.Lock()
			agora := time.Now()
			for k, v := range l.falhas {
				vivos := v[:0]
				for _, t := range v {
					if agora.Sub(t) < janelaFalhas {
						vivos = append(vivos, t)
					}
				}
				if len(vivos) == 0 {
					delete(l.falhas, k)
				} else {
					l.falhas[k] = vivos
				}
			}
			l.mu.Unlock()
		}
	}()
	return l
}

const (
	limiteFalhasLogin = 5  // por login+IP, janela de 15 min (revisão TAKEDA)
	limiteFalhasIP    = 20 // por IP (varredura de logins)
	janelaFalhas      = 15 * time.Minute
	penalidade        = 750 * time.Millisecond
)

func (l *Limiter) permite(chave string, limite int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	agora := time.Now()
	vivos := l.falhas[chave][:0]
	for _, t := range l.falhas[chave] {
		if agora.Sub(t) < janelaFalhas {
			vivos = append(vivos, t)
		}
	}
	l.falhas[chave] = vivos
	return len(vivos) < limite
}

func (l *Limiter) falhou(chave string) {
	l.mu.Lock()
	l.falhas[chave] = append(l.falhas[chave], time.Now())
	l.mu.Unlock()
}

// ---------- requisição ----------

func usuarioDoCtx(r *http.Request) *Usuario {
	u, _ := r.Context().Value(ctxUsuario).(*Usuario)
	return u
}

func ipDe(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// origemConfere valida Origin/Referer (2ª camada anti-CSRF, revisão TAKEDA §11).
func origemConfere(r *http.Request) bool {
	origem := r.Header.Get("Origin")
	if origem == "" {
		origem = r.Header.Get("Referer")
	}
	if origem == "" {
		return true // cliente sem Origin (curl local do operador) — X-SCI já exige
	}
	hosp := r.Host
	return strings.Contains(origem, hosp)
}

// auth exige sessão válida; admin=true exige papel admin; métodos de escrita
// exigem header X-SCI:1 + Origin da própria origem (CSRF).
func (a *App) auth(admin bool, prox http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
			if !origemConfere(r) {
				http.Error(w, `{"erro":"origem estranha"}`, http.StatusForbidden)
				return
			}
			if r.Header.Get("X-SCI") != "1" {
				http.Error(w, `{"erro":"cabeçalho ausente"}`, http.StatusForbidden)
				return
			}
		}
		c, err := r.Cookie(cookieSessao)
		if err != nil || c.Value == "" {
			http.Error(w, `{"erro":"não autenticado"}`, http.StatusUnauthorized)
			return
		}
		u, err := a.st.UsuarioDaSessao(c.Value)
		if err != nil {
			http.Error(w, `{"erro":"sessão inválida"}`, http.StatusInternalServerError)
			return
		}
		if u == nil {
			http.Error(w, `{"erro":"sessão expirada"}`, http.StatusUnauthorized)
			return
		}
		if admin && u.Papel != "admin" {
			http.Error(w, `{"erro":"restrito ao admin"}`, http.StatusForbidden)
			return
		}
		prox(w, r.WithContext(context.WithValue(r.Context(), ctxUsuario, u)))
	})
}

// authPapeis: exige sessão e papel na lista (p.ex. conferência: gerente|operador).
func (a *App) authPapeis(papeis []string, prox http.HandlerFunc) http.Handler {
	return a.auth(false, func(w http.ResponseWriter, r *http.Request) {
		p := usuarioDoCtx(r).Papel
		for _, want := range papeis {
			if p == want {
				prox(w, r)
				return
			}
		}
		http.Error(w, `{"erro":"papel sem acesso a esta área"}`, http.StatusForbidden)
	})
}

func (a *App) validarCredenciais(login, senha, ip string) (*Usuario, error) {
	login = strings.ToLower(strings.TrimSpace(login))
	chaveLogin := "LOGIN|" + ip + "|" + login
	chaveIP := "IP|" + ip
	if !a.lim.permite(chaveIP, limiteFalhasIP) || !a.lim.permite(chaveLogin, limiteFalhasLogin) {
		a.st.Auditoria(nil, "login_bloqueado", "usuarios", nil, "login="+login, ip)
		return nil, fmt.Errorf("muitas tentativas; aguarde alguns minutos")
	}
	var (
		id       int64
		papel    string
		pessoaID *int64
		hash     string
		ativo    int
	)
	err := a.st.db.QueryRow(
		`SELECT id, papel, pessoa_id, senha_hash, ativo FROM usuarios WHERE login = ?`, login).
		Scan(&id, &papel, &pessoaID, &hash, &ativo)
	if err != nil || ativo != 1 || !verificaSenha(senha, hash) {
		a.lim.falhou(chaveIP)
		a.lim.falhou(chaveLogin)
		time.Sleep(penalidade)
		a.st.Auditoria(nil, "login_fail", "usuarios", nil, "login="+login, ip)
		return nil, fmt.Errorf("credenciais inválidas")
	}
	a.st.Auditoria(&id, "login_ok", "usuarios", &id, "", ip)
	// perfil completo do usuário (aba Meu usuário — ordem Tenente 28/09 noite)
	var grupoID *int64
	var ng, nc string
	var setorID, funcaoID *int64
	if err := a.st.db.QueryRow(`SELECT grupo_id, COALESCE(nome_guerra,''), COALESCE(nome_completo,''),
		setor_id, funcao_id FROM usuarios WHERE id = ?`, id).
		Scan(&grupoID, &ng, &nc, &setorID, &funcaoID); err != nil {
		return nil, err
	}
	return &Usuario{ID: id, Login: login, Papel: papel, PessoaID: pessoaID, GrupoID: grupoID,
		NomeGuerra: ng, NomeCompleto: nc, SetorID: setorID, FuncaoID: funcaoID}, nil
}
