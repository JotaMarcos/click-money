package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func TestLoadDefaultsForDevelopment(t *testing.T) {
	t.Parallel()
	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env != EnvDevelopment || cfg.IsProduction() {
		t.Errorf("Env = %s", cfg.Env)
	}
	if cfg.HTTPAddr != ":8080" || cfg.DBDriver != DriverSQLite || cfg.DBURL != "click_money.db" {
		t.Errorf("padrões inesperados: %+v", cfg)
	}
	if len(cfg.AllowedOrigins) != 1 || cfg.AllowedOrigins[0] != "http://localhost:5173" {
		t.Errorf("AllowedOrigins = %v", cfg.AllowedOrigins)
	}
	if cfg.LogLevel != slog.LevelInfo || cfg.MaxBodyBytes != 1<<20 || cfg.ShutdownTimeout != 20*time.Second {
		t.Errorf("padrões inesperados: %+v", cfg)
	}
}

func TestLoadProduction(t *testing.T) {
	t.Parallel()
	cfg, err := Load(env(map[string]string{
		"CM_ENV":              "production",
		"CM_HTTP_ADDR":        ":9000",
		"CM_DB_DRIVER":        "postgres",
		"CM_DB_URL":           "postgres://app@db:5432/click_money",
		"CM_MONGO_URL":        "mongodb://mongo:27017",
		"CM_ALLOWED_ORIGINS":  "https://app.exemplo.com.br/, https://outro.exemplo.com.br",
		"CM_LOG_LEVEL":        "warn",
		"CM_MAX_BODY_BYTES":   "2048",
		"CM_SHUTDOWN_TIMEOUT": "5s",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.IsProduction() || cfg.HTTPAddr != ":9000" || cfg.DBDriver != DriverPostgres {
		t.Errorf("config inesperada: %+v", cfg)
	}
	want := []string{"https://app.exemplo.com.br", "https://outro.exemplo.com.br"}
	if len(cfg.AllowedOrigins) != 2 || cfg.AllowedOrigins[0] != want[0] || cfg.AllowedOrigins[1] != want[1] {
		t.Errorf("AllowedOrigins = %v, quero %v", cfg.AllowedOrigins, want)
	}
	if cfg.LogLevel != slog.LevelWarn || cfg.MaxBodyBytes != 2048 || cfg.ShutdownTimeout != 5*time.Second {
		t.Errorf("config inesperada: %+v", cfg)
	}
	if cfg.MongoURL != "mongodb://mongo:27017" {
		t.Errorf("MongoURL = %s", cfg.MongoURL)
	}
}

func TestLoadFailsFastNamingTheVariable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		vars map[string]string
		want string
	}{
		{"ambiente desconhecido", map[string]string{"CM_ENV": "prod"}, "CM_ENV"},
		{"driver desconhecido", map[string]string{"CM_DB_DRIVER": "mysql"}, "CM_DB_DRIVER"},
		{"postgres sem URL", map[string]string{"CM_DB_DRIVER": "postgres"}, "CM_DB_URL é obrigatória"},
		{"URL de outro banco", map[string]string{"CM_DB_DRIVER": "postgres", "CM_DB_URL": "mysql://x"}, "CM_DB_URL deve ser"},
		{"nível de log inválido", map[string]string{"CM_LOG_LEVEL": "verbose"}, "CM_LOG_LEVEL"},
		{"limite de corpo não numérico", map[string]string{"CM_MAX_BODY_BYTES": "muito"}, "CM_MAX_BODY_BYTES"},
		{"limite de corpo fora da faixa", map[string]string{"CM_MAX_BODY_BYTES": "1"}, "CM_MAX_BODY_BYTES"},
		{"duração inválida", map[string]string{"CM_SHUTDOWN_TIMEOUT": "10"}, "CM_SHUTDOWN_TIMEOUT"},
		{"origem com caminho", map[string]string{"CM_ALLOWED_ORIGINS": "http://localhost:5173/app"}, "CM_ALLOWED_ORIGINS"},
		{"origem sem esquema", map[string]string{"CM_ALLOWED_ORIGINS": "localhost:5173"}, "CM_ALLOWED_ORIGINS"},
		{"produção com sqlite", map[string]string{"CM_ENV": "production", "CM_ALLOWED_ORIGINS": "https://a.b"}, "CM_DB_DRIVER deve ser postgres"},
		{
			"produção sem origens",
			map[string]string{"CM_ENV": "production", "CM_DB_DRIVER": "postgres", "CM_DB_URL": "postgres://x/y"},
			"CM_ALLOWED_ORIGINS é obrigatória",
		},
		{
			"produção com origem http",
			map[string]string{"CM_ENV": "staging", "CM_DB_DRIVER": "postgres", "CM_DB_URL": "postgres://x/y", "CM_ALLOWED_ORIGINS": "http://a.b"},
			"só aceita origens https",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := Load(env(tc.vars))
			if err == nil {
				t.Fatal("esperava erro de configuração")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("erro %q não menciona %q", err, tc.want)
			}
		})
	}
}

func TestLoadReportsAllProblemsAtOnce(t *testing.T) {
	t.Parallel()
	_, err := Load(env(map[string]string{"CM_ENV": "x", "CM_LOG_LEVEL": "y", "CM_DB_DRIVER": "z"}))
	if err == nil {
		t.Fatal("esperava erro")
	}
	for _, key := range []string{"CM_ENV", "CM_LOG_LEVEL", "CM_DB_DRIVER"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("erro não menciona %s: %v", key, err)
		}
	}
}

func TestParseDotEnv(t *testing.T) {
	t.Parallel()
	vars, err := ParseDotEnv(strings.NewReader(`
# comentário
CM_HTTP_ADDR=:9090
CM_DB_URL = "dev.db"
CM_LOG_LEVEL='debug'
VAZIA=
COM_IGUAL=a=b
`))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"CM_HTTP_ADDR": ":9090", "CM_DB_URL": "dev.db", "CM_LOG_LEVEL": "debug", "VAZIA": "", "COM_IGUAL": "a=b"}
	if len(vars) != len(want) {
		t.Fatalf("vars = %v", vars)
	}
	for k, v := range want {
		if vars[k] != v {
			t.Errorf("%s = %q, quero %q", k, vars[k], v)
		}
	}

	for _, bad := range []string{"SEM_IGUAL", "=valor"} {
		if _, err := ParseDotEnv(strings.NewReader(bad)); err == nil {
			t.Errorf("ParseDotEnv(%q) deveria falhar", bad)
		}
	}
}

func TestWithFallbackPrefersRealEnvironment(t *testing.T) {
	t.Parallel()
	getenv := WithFallback(env(map[string]string{"A": "real"}), map[string]string{"A": "arquivo", "B": "arquivo"})
	if getenv("A") != "real" || getenv("B") != "arquivo" || getenv("C") != "" {
		t.Errorf("A=%q B=%q C=%q", getenv("A"), getenv("B"), getenv("C"))
	}
}
