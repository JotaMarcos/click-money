// Package db abre o banco relacional e implementa o Unit of Work.
//
// Os dois dialetos (PostgreSQL em produção, SQLite em desenvolvimento) são
// acessados por database/sql: o PostgreSQL pelo driver pgx e o SQLite pelo
// driver modernc, que dispensa CGO. Toda leitura e escrita de negócio
// acontece dentro de uma transação aberta por WithinTenant, que no
// PostgreSQL define app.tenant_id para as políticas de Row-Level Security.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // driver "pgx" do database/sql
	_ "modernc.org/sqlite"             // driver "sqlite" do database/sql

	"github.com/JotaMarcos/click-money/internal/platform/kernel"
)

// Dialect identifica o banco em uso.
type Dialect string

// Dialetos suportados.
const (
	Postgres Dialect = "postgres"
	SQLite   Dialect = "sqlite"
)

// timeLayout é o formato de instantes no SQLite: UTC, largura fixa e
// microssegundos, de modo que a ordem do texto seja a ordem do tempo.
const timeLayout = "2006-01-02T15:04:05.000000Z"

// Rebind converte os marcadores "?" de uma query para o formato do dialeto
// ("$1", "$2"... no PostgreSQL). As queries do sistema não usam "?" literal.
func (d Dialect) Rebind(query string) string {
	if d != Postgres {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 8)
	n := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(query[i])
	}
	return b.String()
}

// TimeArg converte um instante para o valor aceito pelo dialeto como
// parâmetro de query.
func (d Dialect) TimeArg(t time.Time) any {
	if d == SQLite {
		return t.UTC().Format(timeLayout)
	}
	return t.UTC()
}

// Time lê uma coluna de instante em qualquer um dos dialetos. O valor zero
// com Valid falso representa NULL.
type Time struct {
	Time  time.Time
	Valid bool
}

// Scan implementa sql.Scanner.
func (t *Time) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*t = Time{}
		return nil
	case time.Time:
		*t = Time{Time: v.UTC(), Valid: true}
		return nil
	case string:
		return t.parse(v)
	case []byte:
		return t.parse(string(v))
	default:
		return fmt.Errorf("db: tipo %T não é um instante", src)
	}
}

func (t *Time) parse(s string) error {
	parsed, err := time.Parse(timeLayout, s)
	if err != nil {
		return fmt.Errorf("db: instante %q fora do formato esperado: %w", s, err)
	}
	*t = Time{Time: parsed, Valid: true}
	return nil
}

// DB é a conexão com o banco relacional.
type DB struct {
	sql     *sql.DB
	dialect Dialect
}

// Open abre o banco indicado por driver ("postgres" ou "sqlite") e confirma
// que ele responde. No SQLite, dsn é o caminho do arquivo, ou ":memory:".
func Open(ctx context.Context, driver, dsn string) (*DB, error) {
	var (
		conn    *sql.DB
		dialect Dialect
		err     error
	)
	switch driver {
	case string(Postgres):
		dialect = Postgres
		conn, err = sql.Open("pgx", dsn)
		if err == nil {
			conn.SetMaxOpenConns(20)
			conn.SetMaxIdleConns(5)
			conn.SetConnMaxIdleTime(5 * time.Minute)
			conn.SetConnMaxLifetime(30 * time.Minute)
		}
	case string(SQLite):
		dialect = SQLite
		conn, err = sql.Open("sqlite", sqliteDSN(dsn))
		if err == nil {
			// O SQLite aceita um escritor por vez. Uma única conexão evita
			// erros de bloqueio e mantém vivo um banco em memória.
			conn.SetMaxOpenConns(1)
		}
	default:
		return nil, fmt.Errorf("db: driver desconhecido %q", driver)
	}
	if err != nil {
		return nil, fmt.Errorf("db: abrir %s: %w", driver, err)
	}

	d := &DB{sql: conn, dialect: dialect}
	if err := d.Ping(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return d, nil
}

// sqliteDSN acrescenta os pragmas obrigatórios: WAL, chaves estrangeiras
// ligadas (desligadas por padrão no SQLite) e espera em caso de bloqueio.
func sqliteDSN(path string) string {
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(ON)")
	q.Add("_pragma", "busy_timeout(5000)")
	if path == ":memory:" {
		return "file::memory:?" + q.Encode()
	}
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(NORMAL)")
	return "file:" + path + "?" + q.Encode()
}

// Dialect devolve o dialeto do banco aberto.
func (d *DB) Dialect() Dialect { return d.dialect }

// SQL expõe a conexão database/sql, para uso das migrações.
func (d *DB) SQL() *sql.DB { return d.sql }

// Close fecha as conexões.
func (d *DB) Close() error { return d.sql.Close() }

// Ping verifica se o banco responde. É a verificação de prontidão.
func (d *DB) Ping(ctx context.Context) error {
	if err := d.sql.PingContext(ctx); err != nil {
		return fmt.Errorf("db: banco %s não responde: %w", d.dialect, err)
	}
	return nil
}

// Executor é o que um repositório precisa para executar SQL. É satisfeito
// pela transação aberta pelo Unit of Work.
type Executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type txKey struct{}

// ErrNoTransaction indica que um repositório foi chamado fora de um Unit of
// Work. É erro de programação: todo acesso a dados acontece em transação.
var ErrNoTransaction = errors.New("db: nenhuma transação aberta no contexto")

// ErrNestedTransaction indica tentativa de abrir um Unit of Work dentro de
// outro. Cada caso de uso tem exatamente uma transação.
var ErrNestedTransaction = errors.New("db: já existe uma transação aberta no contexto")

// Tx devolve a transação aberta pelo Unit of Work no contexto.
func Tx(ctx context.Context) (Executor, error) {
	tx, ok := ctx.Value(txKey{}).(*sql.Tx)
	if !ok {
		return nil, ErrNoTransaction
	}
	return tx, nil
}

// WithinTenant executa fn em uma transação restrita ao comércio informado.
//
// No PostgreSQL a transação define app.tenant_id, de que dependem as
// políticas de Row-Level Security. Se fn devolver erro ou entrar em pânico,
// a transação é desfeita; caso contrário, é confirmada.
func (d *DB) WithinTenant(ctx context.Context, tenant kernel.TenantID, fn func(ctx context.Context) error) error {
	if tenant.IsZero() {
		return errors.New("db: transação de negócio sem comércio")
	}
	return d.within(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if d.dialect == Postgres {
			// set_config(..., true) equivale a SET LOCAL e aceita parâmetro.
			if _, err := tx.ExecContext(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenant.String()); err != nil {
				return fmt.Errorf("db: definir comércio da transação: %w", err)
			}
		}
		return fn(ctx)
	})
}

// WithinSystem executa fn em uma transação sem comércio definido.
//
// Serve a operações que não pertencem a um comércio: identidade, sessões e a
// leitura da outbox pelo worker. No PostgreSQL, tabelas de negócio continuam
// inacessíveis nessa transação para o papel da aplicação.
func (d *DB) WithinSystem(ctx context.Context, fn func(ctx context.Context) error) error {
	return d.within(ctx, func(ctx context.Context, _ *sql.Tx) error { return fn(ctx) })
}

func (d *DB) within(ctx context.Context, fn func(ctx context.Context, tx *sql.Tx) error) (err error) {
	if _, open := ctx.Value(txKey{}).(*sql.Tx); open {
		return ErrNestedTransaction
	}
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db: abrir transação: %w", err)
	}

	committed := false
	defer func() {
		if committed {
			return
		}
		// Cobre erro e pânico. O erro do rollback só importa se não houver
		// outro mais relevante para devolver.
		if rbErr := tx.Rollback(); rbErr != nil && err == nil && !errors.Is(rbErr, sql.ErrTxDone) {
			err = fmt.Errorf("db: desfazer transação: %w", rbErr)
		}
	}()

	if err := fn(context.WithValue(ctx, txKey{}, tx), tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db: confirmar transação: %w", err)
	}
	committed = true
	return nil
}
