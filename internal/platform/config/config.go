// Package config lê e valida a configuração do processo a partir de
// variáveis de ambiente.
//
// A validação acontece toda na inicialização: uma variável ausente ou
// inválida impede o processo de subir e é nomeada na mensagem de erro.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Ambientes aceitos em CM_ENV.
const (
	EnvDevelopment = "development"
	EnvStaging     = "staging"
	EnvProduction  = "production"
)

// Drivers aceitos em CM_DB_DRIVER.
const (
	DriverSQLite   = "sqlite"
	DriverPostgres = "postgres"
)

// Config é a configuração validada do processo.
type Config struct {
	// Env é o ambiente de execução.
	Env string
	// HTTPAddr é o endereço de escuta da API, como ":8080".
	HTTPAddr string
	// DBDriver escolhe o banco relacional: sqlite ou postgres.
	DBDriver string
	// DBURL é o caminho do arquivo SQLite ou a URL de conexão do PostgreSQL.
	DBURL string
	// MongoURL é a URL do MongoDB. Vazia significa "sem banco de documentos".
	MongoURL string
	// AllowedOrigins lista as origens aceitas pelo CORS.
	AllowedOrigins []string
	// LogLevel é o nível mínimo de log.
	LogLevel slog.Level
	// MaxBodyBytes limita o corpo das requisições.
	MaxBodyBytes int64
	// ShutdownTimeout é o prazo para terminar requisições em andamento.
	ShutdownTimeout time.Duration
}

// IsProduction informa se o ambiente é de produção ou homologação, onde
// valem as regras mais estritas (logs em JSON, HSTS, sem SQLite).
func (c Config) IsProduction() bool { return c.Env != EnvDevelopment }

// Load monta a configuração a partir de getenv (normalmente os.Getenv).
// Devolve um único erro que reúne todos os problemas encontrados.
func Load(getenv func(string) string) (Config, error) {
	r := reader{getenv: getenv}

	cfg := Config{
		Env:             r.oneOf("CM_ENV", EnvDevelopment, EnvDevelopment, EnvStaging, EnvProduction),
		HTTPAddr:        r.str("CM_HTTP_ADDR", ":8080"),
		DBDriver:        r.oneOf("CM_DB_DRIVER", DriverSQLite, DriverSQLite, DriverPostgres),
		MongoURL:        r.str("CM_MONGO_URL", ""),
		AllowedOrigins:  r.origins("CM_ALLOWED_ORIGINS", "http://localhost:5173"),
		LogLevel:        r.level("CM_LOG_LEVEL", slog.LevelInfo),
		MaxBodyBytes:    r.int64Between("CM_MAX_BODY_BYTES", 1<<20, 1<<10, 16<<20),
		ShutdownTimeout: r.duration("CM_SHUTDOWN_TIMEOUT", 20*time.Second, time.Second, 2*time.Minute),
	}

	switch cfg.DBDriver {
	case DriverSQLite:
		cfg.DBURL = r.str("CM_DB_URL", "click_money.db")
	case DriverPostgres:
		cfg.DBURL = r.required("CM_DB_URL")
		if cfg.DBURL != "" {
			if u, err := url.Parse(cfg.DBURL); err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
				r.fail("CM_DB_URL", "deve ser uma URL postgres://")
			}
		}
	}

	if cfg.IsProduction() {
		if cfg.DBDriver != DriverPostgres {
			r.fail("CM_DB_DRIVER", "deve ser postgres fora do ambiente de desenvolvimento")
		}
		if getenv("CM_ALLOWED_ORIGINS") == "" {
			r.fail("CM_ALLOWED_ORIGINS", "é obrigatória fora do ambiente de desenvolvimento")
		}
		for _, o := range cfg.AllowedOrigins {
			if !strings.HasPrefix(o, "https://") {
				r.fail("CM_ALLOWED_ORIGINS", "só aceita origens https fora do ambiente de desenvolvimento")
				break
			}
		}
	}

	if len(r.errs) > 0 {
		return Config{}, fmt.Errorf("configuração inválida: %w", errors.Join(r.errs...))
	}
	return cfg, nil
}

// ParseDotEnv lê pares CHAVE=valor de um arquivo .env. Linhas vazias e
// iniciadas por "#" são ignoradas; aspas simples ou duplas em volta do valor
// são removidas. Não há expansão de variáveis.
func ParseDotEnv(r io.Reader) (map[string]string, error) {
	vars := make(map[string]string)
	sc := bufio.NewScanner(r)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		key, value, ok := strings.Cut(text, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("config: linha %d do .env não está no formato CHAVE=valor", line)
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}
		vars[key] = value
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("config: ler .env: %w", err)
	}
	return vars, nil
}

// WithFallback devolve um getenv que consulta primeiro o ambiente real e,
// se a variável estiver vazia, o mapa informado (o conteúdo do .env).
func WithFallback(getenv func(string) string, fallback map[string]string) func(string) string {
	return func(key string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return fallback[key]
	}
}

// reader acumula os erros de leitura para que todos apareçam de uma vez.
type reader struct {
	getenv func(string) string
	errs   []error
}

func (r *reader) fail(key, problem string) {
	r.errs = append(r.errs, fmt.Errorf("%s %s", key, problem))
}

func (r *reader) str(key, def string) string {
	if v := strings.TrimSpace(r.getenv(key)); v != "" {
		return v
	}
	return def
}

func (r *reader) required(key string) string {
	v := strings.TrimSpace(r.getenv(key))
	if v == "" {
		r.fail(key, "é obrigatória")
	}
	return v
}

func (r *reader) oneOf(key, def string, allowed ...string) string {
	v := r.str(key, def)
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	r.fail(key, "deve ser um de: "+strings.Join(allowed, ", "))
	return def
}

func (r *reader) int64Between(key string, def, lo, hi int64) int64 {
	raw := strings.TrimSpace(r.getenv(key))
	if raw == "" {
		return def
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v < lo || v > hi {
		r.fail(key, fmt.Sprintf("deve ser um inteiro entre %d e %d", lo, hi))
		return def
	}
	return v
}

func (r *reader) duration(key string, def, lo, hi time.Duration) time.Duration {
	raw := strings.TrimSpace(r.getenv(key))
	if raw == "" {
		return def
	}
	v, err := time.ParseDuration(raw)
	if err != nil || v < lo || v > hi {
		r.fail(key, fmt.Sprintf("deve ser uma duração entre %s e %s", lo, hi))
		return def
	}
	return v
}

func (r *reader) level(key string, def slog.Level) slog.Level {
	raw := strings.TrimSpace(r.getenv(key))
	if raw == "" {
		return def
	}
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(raw)); err != nil {
		r.fail(key, "deve ser debug, info, warn ou error")
		return def
	}
	return lvl
}

func (r *reader) origins(key, def string) []string {
	var out []string
	for _, item := range strings.Split(r.str(key, def), ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		u, err := url.Parse(item)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || strings.TrimSuffix(u.Path, "/") != "" {
			r.fail(key, fmt.Sprintf("contém origem inválida %q (use esquema://host[:porta])", item))
			continue
		}
		out = append(out, u.Scheme+"://"+u.Host)
	}
	return out
}
