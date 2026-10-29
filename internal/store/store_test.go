package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/aam9063/hipoteca-cli/internal/errs"
)

// openTemp abre una base de datos en un directorio temporal del test y la
// cierra al terminar. Ningún test toca la base real del usuario.
func openTemp(t *testing.T) (*DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hipoteca.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, path
}

// TestOpenCreaDirectorioYFicheroConPermisos: el directorio queda a 0700 y la
// base a 0600, la misma disciplina que la config (decisión 1 de WATCH-01).
func TestOpenCreaDirectorioYFicheroConPermisos(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("los permisos POSIX no aplican en Windows")
	}
	base := t.TempDir()
	path := filepath.Join(base, "datos", "hipoteca.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	defer db.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat de la base: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("permisos de la base = %o, want 600", got)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat del directorio: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Errorf("permisos del directorio = %o, want 700", got)
	}
}

// TestOpenIncapazDeAbrirDaErrorClaro: una ruta imposible (un fichero donde
// debería estar el directorio) produce un error del contrato (internal/1) que
// nombra la ruta, no un panic ni un error pelado.
func TestOpenIncapazDeAbrirDaErrorClaro(t *testing.T) {
	dir := t.TempDir()
	bloqueo := filepath.Join(dir, "fichero")
	if err := os.WriteFile(bloqueo, []byte("x"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	path := filepath.Join(bloqueo, "hipoteca.db") // bloqueo es un fichero, no un directorio

	_, err := Open(path)
	if err == nil {
		t.Fatal("Open sobre una ruta imposible: se esperaba error")
	}
	env, ok := errs.From(err)
	if !ok {
		t.Fatalf("el error debe llevar el sobre del contrato, got %v", err)
	}
	if env.Error.ExitCode != 1 {
		t.Errorf("código = %d, want 1", env.Error.ExitCode)
	}
	if !strings.Contains(err.Error(), "hipoteca.db") {
		t.Errorf("el error debe nombrar la ruta, got %q", err.Error())
	}
}

// TestAddWatchRoundTrip: lo guardado se lee igual tras reabrir la base
// (WATCH-01.1).
func TestAddWatchRoundTrip(t *testing.T) {
	_, path := openTemp(t)
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

	var added Watch
	{
		db, err := Open(path)
		if err != nil {
			t.Fatalf("reabrir: %v", err)
		}
		added, err = db.AddWatch("index", "euribor-1y", "webhook", true, now)
		if err != nil {
			t.Fatalf("AddWatch: %v", err)
		}
		if added.ID == 0 {
			t.Fatal("AddWatch devolvió un watch sin id")
		}
		if err := db.Close(); err != nil {
			t.Fatalf("cerrar: %v", err)
		}
	}

	db, err := Open(path)
	if err != nil {
		t.Fatalf("reabrir: %v", err)
	}
	defer db.Close()
	watches, err := db.Watches()
	if err != nil {
		t.Fatalf("Watches: %v", err)
	}
	if len(watches) != 1 {
		t.Fatalf("Watches = %d elementos, want 1", len(watches))
	}
	w := watches[0]
	if w.ID != added.ID || w.Kind != "index" || w.Target != "euribor-1y" || w.Sink != "webhook" || !w.WithImpact {
		t.Errorf("watch leído = %+v, want el guardado", w)
	}
	if !w.CreatedAt.Equal(now) {
		t.Errorf("created_at = %v, want %v", w.CreatedAt, now)
	}
}

// TestAddWatchDuplicadoRechazado: repetir `watch add` sobre el mismo target no
// crea una segunda vigilancia (decisión 2: un target, una fila).
func TestAddWatchDuplicadoRechazado(t *testing.T) {
	db, _ := openTemp(t)
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

	if _, err := db.AddWatch("index", "euribor-1y", "webhook", false, now); err != nil {
		t.Fatalf("primer AddWatch: %v", err)
	}
	_, err := db.AddWatch("index", "euribor-1y", "webhook", false, now)
	if err == nil {
		t.Fatal("AddWatch duplicado: se esperaba error")
	}
	assertCode(t, err, 2)
	watches, err := db.Watches()
	if err != nil {
		t.Fatalf("Watches: %v", err)
	}
	if len(watches) != 1 {
		t.Fatalf("Watches = %d elementos, want 1 (el duplicado no debe crearse)", len(watches))
	}
}

// TestRemoveWatchBorraTambiénSusMesesVistos: quitar la vigilancia no deja
// restos de su estado.
func TestRemoveWatchBorraTambiénSusMesesVistos(t *testing.T) {
	db, _ := openTemp(t)
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	w, err := db.AddWatch("index", "euribor-1y", "webhook", false, now)
	if err != nil {
		t.Fatalf("AddWatch: %v", err)
	}
	if inserted, err := db.RecordSeen(w.ID, "2026-09", "2.383", now); err != nil || !inserted {
		t.Fatalf("RecordSeen: inserted=%v err=%v", inserted, err)
	}

	if err := db.RemoveWatch(w.ID); err != nil {
		t.Fatalf("RemoveWatch: %v", err)
	}
	watches, err := db.Watches()
	if err != nil {
		t.Fatalf("Watches: %v", err)
	}
	if len(watches) != 0 {
		t.Fatalf("Watches = %d elementos, want 0", len(watches))
	}
}

// TestRemoveWatchInexistenteEsNotFound: un id que no existe es not_found/4.
func TestRemoveWatchInexistenteEsNotFound(t *testing.T) {
	db, _ := openTemp(t)
	err := db.RemoveWatch(999)
	if err == nil {
		t.Fatal("RemoveWatch de un id inexistente: se esperaba error")
	}
	assertCode(t, err, 4)
}

// TestRecordSeenEsIdempotente: la clave (watch, mes) evita duplicar; una
// segunda grabación del mismo mes no inserta nada (decisión 2).
func TestRecordSeenEsIdempotente(t *testing.T) {
	db, _ := openTemp(t)
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	w, err := db.AddWatch("index", "euribor-1y", "webhook", false, now)
	if err != nil {
		t.Fatalf("AddWatch: %v", err)
	}

	inserted, err := db.RecordSeen(w.ID, "2026-10", "2.211", now)
	if err != nil || !inserted {
		t.Fatalf("primera RecordSeen: inserted=%v err=%v", inserted, err)
	}
	seenAt2 := now.Add(time.Hour)
	inserted, err = db.RecordSeen(w.ID, "2026-10", "2.211", seenAt2)
	if err != nil {
		t.Fatalf("segunda RecordSeen: %v", err)
	}
	if inserted {
		t.Error("la segunda RecordSeen del mismo mes dice inserted=true; want false")
	}
	seen, err := db.SeenMonths(w.ID)
	if err != nil {
		t.Fatalf("SeenMonths: %v", err)
	}
	if len(seen) != 1 {
		t.Fatalf("SeenMonths = %d filas, want 1 (repetir el check no duplica)", len(seen))
	}
	if seen[0].Month != "2026-10" || seen[0].Value != "2.211" {
		t.Errorf("fila = %+v, want mes 2026-10 valor 2.211", seen[0])
	}
	if !seen[0].SeenAt.Equal(now) {
		t.Errorf("seen_at = %v, want %v (la segunda grabación no pisa la primera)", seen[0].SeenAt, now)
	}
}

// TestSeenMonthsEnOrdenAscendente.
func TestSeenMonthsEnOrdenAscendente(t *testing.T) {
	db, _ := openTemp(t)
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	w, err := db.AddWatch("index", "euribor-1y", "webhook", false, now)
	if err != nil {
		t.Fatalf("AddWatch: %v", err)
	}
	for _, m := range []string{"2026-08", "2026-10", "2026-09"} {
		if _, err := db.RecordSeen(w.ID, m, "2.2", now); err != nil {
			t.Fatalf("RecordSeen(%s): %v", m, err)
		}
	}
	seen, err := db.SeenMonths(w.ID)
	if err != nil {
		t.Fatalf("SeenMonths: %v", err)
	}
	want := []string{"2026-08", "2026-09", "2026-10"}
	for i, s := range seen {
		if s.Month != want[i] {
			t.Errorf("seen[%d].Month = %s, want %s", i, s.Month, want[i])
		}
	}
}

// TestMarkDelivered: marcar la entrega deja delivered_at; el mes grabado se
// conserva (decisión 5: el evento no se borra por un fallo del sink).
func TestMarkDelivered(t *testing.T) {
	db, _ := openTemp(t)
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	w, err := db.AddWatch("index", "euribor-1y", "webhook", false, now)
	if err != nil {
		t.Fatalf("AddWatch: %v", err)
	}
	if _, err := db.RecordSeen(w.ID, "2026-10", "2.211", now); err != nil {
		t.Fatalf("RecordSeen: %v", err)
	}

	delivered := now.Add(30 * time.Minute)
	if err := db.MarkDelivered(w.ID, "2026-10", delivered); err != nil {
		t.Fatalf("MarkDelivered: %v", err)
	}
	seen, err := db.SeenMonths(w.ID)
	if err != nil {
		t.Fatalf("SeenMonths: %v", err)
	}
	if len(seen) != 1 {
		t.Fatalf("SeenMonths = %d filas, want 1", len(seen))
	}
	if seen[0].DeliveredAt == nil || !seen[0].DeliveredAt.Equal(delivered) {
		t.Errorf("delivered_at = %v, want %v", seen[0].DeliveredAt, delivered)
	}
}

// assertCode comprueba que el error lleva el sobre del contrato con el código
// estable esperado.
func assertCode(t *testing.T, err error, code int) {
	t.Helper()
	env, ok := errs.From(err)
	if !ok {
		t.Fatalf("el error debe llevar el sobre del contrato, got %v", err)
	}
	if env.Error.ExitCode != code {
		t.Fatalf("código = %d, want %d (mensaje: %s)", env.Error.ExitCode, code, env.Error.Message)
	}
}

// TestOpenFijaLaVersionDeEsquemaEnBaseNueva (Fix 3): una base recién creada
// queda marcada con user_version 1, la versión de este esquema.
func TestOpenFijaLaVersionDeEsquemaEnBaseNueva(t *testing.T) {
	_, path := openTemp(t)
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("abrir la base para leer user_version: %v", err)
	}
	defer raw.Close()
	var version int
	if err := raw.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("leer user_version: %v", err)
	}
	if version != 1 {
		t.Errorf("user_version = %d, want 1", version)
	}
}

// TestOpenRechazaVersionDeEsquemaMasNueva (Fix 3): una base escrita con un
// esquema más nuevo se rechaza con un error que nombra el fichero y la
// versión; nunca se adopta en silencio.
func TestOpenRechazaVersionDeEsquemaMasNueva(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hipoteca.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("crear la base de prueba: %v", err)
	}
	if _, err := raw.Exec(`CREATE TABLE watches (id INTEGER); PRAGMA user_version = 2`); err != nil {
		t.Fatalf("preparar la base de prueba: %v", err)
	}
	_ = raw.Close()

	_, err = Open(path)
	if err == nil {
		t.Fatal("Open de una base con user_version 2: se esperaba error")
	}
	assertCode(t, err, 1)
	for _, want := range []string{"hipoteca.db", "2"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("el error debe nombrar %q, got %q", want, err.Error())
		}
	}
}

// TestOpenRechazaSQLiteAjenoSinVersion (Fix 3): un SQLite válido de otro
// programa (tablas, user_version 0) no se adopta como base del programa.
func TestOpenRechazaSQLiteAjenoSinVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hipoteca.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("crear la base de prueba: %v", err)
	}
	if _, err := raw.Exec(`CREATE TABLE favoritos_del_usuario (id INTEGER)`); err != nil {
		t.Fatalf("preparar la base de prueba: %v", err)
	}
	_ = raw.Close()

	_, err = Open(path)
	if err == nil {
		t.Fatal("Open de un SQLite ajeno: se esperaba error")
	}
	assertCode(t, err, 1)
	if !strings.Contains(err.Error(), "hipoteca.db") {
		t.Errorf("el error debe nombrar el fichero, got %q", err.Error())
	}
}
