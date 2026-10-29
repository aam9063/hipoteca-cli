package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/aam9063/hipoteca-cli/internal/errs"
)

// DB es la base SQLite local del estado de ejecución (decisión 1 de
// WATCH-01): las vigilancias y los meses ya vistos de cada serie. Vive en
// config.DataPath() (arranque §7, `hipoteca.db`, 0600 en un directorio 0700);
// la config TOML sigue siendo el sitio de los datos del usuario.
//
// El estado aquí guardado no se puede reconstruir del todo: si un mes ya se
// vio, volver a avisar sería un falso positivo, así que la escritura es
// idempotente (clave única) y los permisos se imponen como la config.
type DB struct {
	sql *sql.DB
}

// Watch es una vigilancia guardada: en v1 el único kind es "index" y el
// target es el id del catálogo (p. ej. euribor-1y).
type Watch struct {
	ID         int64
	Kind       string
	Target     string
	Sink       string
	WithImpact bool
	CreatedAt  time.Time
}

// Seen es un mes ya visto de una vigilancia: el valor exacto publicado por el
// Banco (P-07) y cuándo se grabó y (si procede) cuándo se entregó al sink. Un
// mes grabado sin entregar no se pierde por un fallo del sink: se reintenta en
// la siguiente pasada (decisión 5).
type Seen struct {
	WatchID     int64
	Month       string // YYYY-MM
	Value       string // decimal exacto tal como lo publicó el Banco
	SeenAt      time.Time
	DeliveredAt *time.Time
}

// stamp es el formato con el que viajan los tiempos a la base: UTC y con
// precisión de nanosegundos, para que leído y escrito coincidan.
const stamp = time.RFC3339Nano

// schema crea las dos tablas si no existen. La clave única de seen_months
// ((watch_id, month)) es la que garantiza la decisión 2: un evento por mes
// nuevo, ni cero ni dos.
const schema = `
CREATE TABLE IF NOT EXISTS watches (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	kind        TEXT NOT NULL,
	target      TEXT NOT NULL,
	sink        TEXT NOT NULL,
	with_impact INTEGER NOT NULL DEFAULT 0,
	created_at  TEXT NOT NULL,
	UNIQUE(kind, target)
);
CREATE TABLE IF NOT EXISTS seen_months (
	watch_id     INTEGER NOT NULL REFERENCES watches(id) ON DELETE CASCADE,
	month        TEXT NOT NULL,
	value        TEXT NOT NULL,
	seen_at      TEXT NOT NULL,
	delivered_at TEXT,
	UNIQUE(watch_id, month)
);
`

// Open abre (o crea) la base en path con la disciplina de la config:
// directorio 0700 y fichero 0600. Un error al abrir sale como internal/1
// nombrando la ruta: sin base no hay estado y el usuario debe saberlo, no
// descubrirlo por un fallo silencioso.
func Open(path string) (*DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, errs.New(errs.KindInternal,
			fmt.Sprintf("no se pudo crear el directorio de datos %s para la base %s: %v", dir, path, err),
			"comprueba los permisos del directorio")
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, errs.New(errs.KindInternal,
				fmt.Sprintf("no se pudo crear la base de datos %s: %v", path, err),
				"comprueba los permisos del directorio")
		}
		_ = f.Close()
	} else if err == nil {
		// Ya existía: se impone el permiso por si lo perdió.
		if err := os.Chmod(path, 0o600); err != nil {
			return nil, errs.New(errs.KindInternal,
				fmt.Sprintf("no se pudo fijar el permiso 0600 de la base de datos %s: %v", path, err), "")
		}
	}
	sqlDB, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, errs.New(errs.KindInternal,
			fmt.Sprintf("no se pudo abrir la base de datos %s: %v", path, err),
			"comprueba que el fichero no esté corrupto y los permisos")
	}
	// Una única conexión: el estado es local y secuencial; sin concurrencia
	// que justifique un pool.
	sqlDB.SetMaxOpenConns(1)
	if _, err := sqlDB.Exec(schema); err != nil {
		_ = sqlDB.Close()
		return nil, errs.New(errs.KindInternal,
			fmt.Sprintf("no se pudo preparar la base de datos %s: %v", path, err), "")
	}
	return &DB{sql: sqlDB}, nil
}

// Close cierra la base.
func (d *DB) Close() error {
	return d.sql.Close()
}

// AddWatch guarda una vigilancia nueva. Repetir kind+target es un error de uso
// (código 2): un target, una vigilancia — añadir otra vez no duplica avisos.
func (d *DB) AddWatch(kind, target, sink string, withImpact bool, now time.Time) (Watch, error) {
	res, err := d.sql.Exec(
		`INSERT INTO watches (kind, target, sink, with_impact, created_at) VALUES (?, ?, ?, ?, ?)`,
		kind, target, sink, boolToInt(withImpact), now.UTC().Format(stamp))
	if err != nil {
		if isUniqueViolation(err) {
			return Watch{}, errs.New(errs.KindUsage,
				fmt.Sprintf("ya existe una vigilancia de %s sobre %q", kind, target),
				"\"hipoteca watch list\" muestra las vigilancias guardadas; no hace falta añadirla dos veces")
		}
		return Watch{}, errs.New(errs.KindInternal, "no se pudo guardar la vigilancia: "+err.Error(), "")
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Watch{}, errs.New(errs.KindInternal, "no se pudo leer el id de la vigilancia: "+err.Error(), "")
	}
	return Watch{
		ID:         id,
		Kind:       kind,
		Target:     target,
		Sink:       sink,
		WithImpact: withImpact,
		CreatedAt:  now.UTC(),
	}, nil
}

// Watches devuelve todas las vigilancias guardadas, por orden de creación.
func (d *DB) Watches() ([]Watch, error) {
	rows, err := d.sql.Query(`SELECT id, kind, target, sink, with_impact, created_at FROM watches ORDER BY id`)
	if err != nil {
		return nil, errs.New(errs.KindInternal, "no se pudieron leer las vigilancias: "+err.Error(), "")
	}
	defer rows.Close()
	var out []Watch
	for rows.Next() {
		var w Watch
		var withImpact int
		var created string
		if err := rows.Scan(&w.ID, &w.Kind, &w.Target, &w.Sink, &withImpact, &created); err != nil {
			return nil, errs.New(errs.KindInternal, "no se pudo leer una vigilancia: "+err.Error(), "")
		}
		w.WithImpact = withImpact != 0
		w.CreatedAt, err = time.Parse(stamp, created)
		if err != nil {
			return nil, errs.New(errs.KindInternal, "la fecha de creación de una vigilancia no es válida: "+err.Error(), "")
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// WatchByID devuelve la vigilancia con ese id. Un id desconocido es
// not_found/4.
func (d *DB) WatchByID(id int64) (Watch, error) {
	row := d.sql.QueryRow(`SELECT id, kind, target, sink, with_impact, created_at FROM watches WHERE id = ?`, id)
	var w Watch
	var withImpact int
	var created string
	switch err := row.Scan(&w.ID, &w.Kind, &w.Target, &w.Sink, &withImpact, &created); {
	case errors.Is(err, sql.ErrNoRows):
		return Watch{}, errs.New(errs.KindNotFound,
			fmt.Sprintf("no hay ninguna vigilancia con el id %d", id),
			"\"hipoteca watch list\" muestra las vigilancias guardadas")
	case err != nil:
		return Watch{}, errs.New(errs.KindInternal, "no se pudo leer la vigilancia: "+err.Error(), "")
	}
	w.WithImpact = withImpact != 0
	created_at, err := time.Parse(stamp, created)
	if err != nil {
		return Watch{}, errs.New(errs.KindInternal, "la fecha de creación de la vigilancia no es válida: "+err.Error(), "")
	}
	w.CreatedAt = created_at
	return w, nil
}

// RemoveWatch borra la vigilancia y sus meses vistos (ON DELETE CASCADE). Un
// id desconocido es not_found/4 y no toca nada.
func (d *DB) RemoveWatch(id int64) error {
	if _, err := d.WatchByID(id); err != nil {
		return err
	}
	if _, err := d.sql.Exec(`DELETE FROM watches WHERE id = ?`, id); err != nil {
		return errs.New(errs.KindInternal, "no se pudo borrar la vigilancia: "+err.Error(), "")
	}
	return nil
}

// SeenMonths devuelve los meses vistos de una vigilancia en orden ascendente
// (el más antiguo primero), con su marca de entrega.
func (d *DB) SeenMonths(watchID int64) ([]Seen, error) {
	rows, err := d.sql.Query(
		`SELECT watch_id, month, value, seen_at, delivered_at FROM seen_months WHERE watch_id = ? ORDER BY month`,
		watchID)
	if err != nil {
		return nil, errs.New(errs.KindInternal, "no se pudieron leer los meses vistos: "+err.Error(), "")
	}
	defer rows.Close()
	var out []Seen
	for rows.Next() {
		var s Seen
		var seenAt string
		var delivered sql.NullString
		if err := rows.Scan(&s.WatchID, &s.Month, &s.Value, &seenAt, &delivered); err != nil {
			return nil, errs.New(errs.KindInternal, "no se pudo leer un mes visto: "+err.Error(), "")
		}
		s.SeenAt, err = time.Parse(stamp, seenAt)
		if err != nil {
			return nil, errs.New(errs.KindInternal, "la fecha de un mes visto no es válida: "+err.Error(), "")
		}
		if delivered.Valid {
			t, err := time.Parse(stamp, delivered.String)
			if err != nil {
				return nil, errs.New(errs.KindInternal, "la fecha de entrega de un mes no es válida: "+err.Error(), "")
			}
			s.DeliveredAt = &t
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// RecordSeen graba el mes visto con su valor exacto. La clave única
// (watch_id, month) hace la escritura idempotente (decisión 2): grabar el
// mismo mes otra vez no inserta nada y devuelve inserted=false; el seen_at de
// la primera grabación no se pisa.
func (d *DB) RecordSeen(watchID int64, month, value string, now time.Time) (bool, error) {
	res, err := d.sql.Exec(
		`INSERT INTO seen_months (watch_id, month, value, seen_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(watch_id, month) DO NOTHING`,
		watchID, month, value, now.UTC().Format(stamp))
	if err != nil {
		return false, errs.New(errs.KindInternal, "no se pudo grabar el mes visto: "+err.Error(), "")
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, errs.New(errs.KindInternal, "no se pudo confirmar la grabación del mes: "+err.Error(), "")
	}
	return n > 0, nil
}

// MarkDelivered deja constancia de que el aviso del mes llegó al sink. El mes
// y su valor no se tocan: la entrega es estado aparte del dato.
func (d *DB) MarkDelivered(watchID int64, month string, now time.Time) error {
	if _, err := d.sql.Exec(
		`UPDATE seen_months SET delivered_at = ? WHERE watch_id = ? AND month = ?`,
		now.UTC().Format(stamp), watchID, month); err != nil {
		return errs.New(errs.KindInternal, "no se pudo marcar la entrega del aviso: "+err.Error(), "")
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// isUniqueViolation reconoce la violación de clave única de SQLite, tanto por
// el código del driver (2067 SQLITE_CONSTRAINT_UNIQUE, 1555
// SQLITE_CONSTRAINT_PRIMARYKEY) como por el mensaje del contrato de SQLite.
func isUniqueViolation(err error) bool {
	var c interface{ Code() int }
	if errors.As(err, &c) && (c.Code() == 2067 || c.Code() == 1555) {
		return true
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
