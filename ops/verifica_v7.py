import sqlite3
con = sqlite3.connect('/home/aimi/projetos/sci/dados/sci.db')
tabs = [r[0] for r in con.execute("SELECT name FROM sqlite_master WHERE type='table'")]
print("grupos:", "grupos" in tabs)
print("comentarios:", "comentarios" in tabs)
print("status_pessoal:", "status_pessoal" in tabs)
print("migr_v4:", con.execute("SELECT COUNT(*) FROM schema_migrations WHERE versao=4").fetchone()[0])
ddl = con.execute("SELECT sql FROM sqlite_master WHERE name='conferencias'").fetchone()[0] or ""
print("sem_unique:", "UNIQUE" not in ddl.upper())
con.close()
