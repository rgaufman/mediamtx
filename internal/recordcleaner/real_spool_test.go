package recordcleaner

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/test"
)

// The fixtures in testdata/ were captured from a production TetherBox (15 Nov 2026):
//   - mediamtx.yml is its MediaMTX configuration with every credential replaced by REDACTED;
//   - spool_listing.json is a names-only listing (relative path, file or dir, age in seconds)
//     of every recording spool directory the box had: tmp/recorder, tmp/spools/cameras,
//     tmp/spools/disk2 and tmp/spools/disk3. Files are rebuilt here as zero-byte files.
//
// The yml records to tmp/spools/cameras/%path/..., a directory that held only stray files at
// capture time. The other three spools hold the flat "<camera id>_<YYYYMMDD_HHMMSS_ffffff>.mp4"
// segments the box really produces, next to .json markers, hourly folders and thumbnails,
// so the pass is also run with recordPath retargeted at each of them.

const realDeployDir = "/data/deployer/tetherbox/"

type listingEntry struct {
	Path string `json:"path"`
	Type string `json:"type"`
	Age  int64  `json:"age_s"`
}

type listing struct {
	CapturedAt int64                     `json:"captured_at"`
	Roots      map[string][]listingEntry `json:"roots"`
}

func loadListing(t *testing.T) listing {
	t.Helper()
	buf, err := os.ReadFile(filepath.Join("testdata", "spool_listing.json"))
	require.NoError(t, err)
	var l listing
	require.NoError(t, json.Unmarshal(buf, &l))
	require.NotEmpty(t, l.Roots)
	return l
}

// rebuildTree recreates the captured tree under dir, with modification times
// shifted so that the captured ages are relative to the current time.
func rebuildTree(t *testing.T, dir string, l listing) {
	t.Helper()
	now := time.Now()
	for root, entries := range l.Roots {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, root), 0o755))
		for _, e := range entries {
			full := filepath.Join(dir, root, filepath.FromSlash(e.Path))
			mtime := now.Add(-time.Duration(e.Age) * time.Second)
			switch e.Type {
			case "dir":
				require.NoError(t, os.MkdirAll(full, 0o755))
			case "file":
				require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
				require.NoError(t, os.WriteFile(full, nil, 0o644))
				require.NoError(t, os.Chtimes(full, mtime, mtime))
			default:
				t.Fatalf("unknown entry type %q", e.Type)
			}
		}
	}
}

// loadConfs loads the captured configuration with the box's deploy directory mapped to dir.
// retargetRoot, when not empty, points the recordPath of every recording path at
// "<dir>/<retargetRoot>/%path_%Y%m%d_%H%M%S_%f".
func loadConfs(t *testing.T, dir string, retargetRoot string) map[string]*conf.Path {
	t.Helper()
	buf, err := os.ReadFile(filepath.Join("testdata", "mediamtx.yml"))
	require.NoError(t, err)

	text := string(buf)
	if retargetRoot != "" {
		old := realDeployDir + "tmp/spools/cameras/%path/%path_"
		require.Contains(t, text, old)
		text = strings.ReplaceAll(text, old, realDeployDir+retargetRoot+"/%path_")
	}
	text = strings.ReplaceAll(text, realDeployDir, dir+"/")

	confPath := filepath.Join(dir, "mediamtx.yml")
	require.NoError(t, os.WriteFile(confPath, []byte(text), 0o644))

	c, _, err := conf.Load(confPath, nil, test.NilLogger)
	require.NoError(t, err)

	recording := 0
	for _, pc := range c.Paths {
		if pc.Record {
			recording++
		}
	}
	require.NotZero(t, recording, "the captured configuration must have recording paths")
	return c.Paths
}

// snapshot maps every path below dir (excluding the config file) to "dir" or "file".
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := make(map[string]string)
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if rel == "." || rel == "mediamtx.yml" {
			return nil
		}
		if d.IsDir() {
			out[filepath.ToSlash(rel)] = "dir"
		} else {
			out[filepath.ToSlash(rel)] = "file"
		}
		return nil
	})
	require.NoError(t, err)
	return out
}

// flatSegmentRe is not anchored at the end on purpose: recordstore.PathDecoder compiles the
// record path without "$", so the cleaner also treats the box's "<segment>.mp4.json" and
// "<segment>.mp4.open.json" sidecars as belonging to the segment and removes them with it.
// That is observed behaviour of the cleaner on the real spool, modelled here as it is.
var flatSegmentRe = regexp.MustCompile(`^([0-9a-f]+)_(\d{8})_(\d{6})_(\d{6})\.mp4`)

// expectedAfterPass is an independent model of what a pass must do to a flat recordPath root:
// remove the "<id>_<time>.mp4" files (and their ".json" sidecars) of recording paths whose
// time (UTC) is not after now - recordDeleteAfter, then remove every directory of the root
// left empty. Everything else, including every other spool, must stay.
func expectedAfterPass(before map[string]string, root string, now time.Time,
	confs map[string]*conf.Path,
) (after map[string]string, removedSegments int) {
	after = make(map[string]string, len(before))
	for k, v := range before {
		after[k] = v
	}

	for rel, typ := range before {
		if typ != "file" || path.Dir(rel) != root {
			continue
		}
		m := flatSegmentRe.FindStringSubmatch(path.Base(rel))
		if m == nil {
			continue
		}
		pc, ok := confs[m[1]]
		if !ok || !pc.Record || pc.RecordDeleteAfter == 0 {
			continue
		}
		start, err := time.ParseInLocation("20060102_150405", m[2]+"_"+m[3], time.UTC)
		if err != nil {
			continue
		}
		micros, _ := strconv.Atoi(m[4])
		start = start.Add(time.Duration(micros) * time.Microsecond)
		if !now.Add(-time.Duration(pc.RecordDeleteAfter)).Before(start) {
			delete(after, rel)
			removedSegments++
		}
	}

	var dirs []string
	for rel, typ := range after {
		if typ == "dir" && (rel == root || strings.HasPrefix(rel, root+"/")) {
			dirs = append(dirs, rel)
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, d := range dirs {
		empty := true
		for rel := range after {
			if strings.HasPrefix(rel, d+"/") {
				empty = false
				break
			}
		}
		if empty {
			delete(after, d)
		}
	}
	return after, removedSegments
}

func requireSameTree(t *testing.T, want, got map[string]string) {
	t.Helper()
	var missing, extra []string
	for k := range want {
		if _, ok := got[k]; !ok {
			missing = append(missing, k)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			extra = append(extra, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 5 {
		missing = missing[:5]
	}
	if len(extra) > 5 {
		extra = extra[:5]
	}
	require.Empty(t, missing, "removed but must have stayed (first 5)")
	require.Empty(t, extra, "stayed but must have been removed (first 5)")
}

// runPassWithReload starts a cleaner whose first pass is held at its start (as a pass over a
// huge spool would be), checks that ReloadPathConfs returns promptly meanwhile, then lets the
// pass run and returns once the tree equals want and has stayed equal for a while.
func runPassWithReload(t *testing.T, dir string, confs, reloadConfs map[string]*conf.Path,
	now time.Time, want map[string]string,
) {
	t.Helper()
	passStarted := make(chan struct{})
	releasePass := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releasePass) }) }
	first := true

	timeNow = func() time.Time {
		// timeNow is called at the start of every pass; hold the first one.
		if first {
			first = false
			close(passStarted)
			<-releasePass
		}
		return now
	}
	defer func() { timeNow = time.Now }()

	c := &Cleaner{PathConfs: confs, Parent: test.NilLogger}
	c.Initialize()
	defer c.Close()
	defer release()

	<-passStarted

	reloaded := make(chan struct{})
	go func() {
		c.ReloadPathConfs(reloadConfs)
		close(reloaded)
	}()
	select {
	case <-reloaded:
	case <-time.After(1 * time.Second):
		t.Fatal("ReloadPathConfs blocked until the cleaning pass ended")
	}

	release()

	deadline := time.Now().Add(20 * time.Second)
	for {
		if reflectEqual(snapshot(t, dir), want) {
			break
		}
		if time.Now().After(deadline) {
			requireSameTree(t, want, snapshot(t, dir))
			t.Fatal("tree did not reach the expected state")
		}
		time.Sleep(50 * time.Millisecond)
	}

	// nothing else may go afterwards.
	time.Sleep(300 * time.Millisecond)
	requireSameTree(t, want, snapshot(t, dir))
}

func reflectEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestCleanerRealSpoolAsCaptured(t *testing.T) {
	// time.Local decodes the "%Y%m%d_%H%M%S" of segment names; the box names them in UTC.
	oldLocal := time.Local
	time.Local = time.UTC
	defer func() { time.Local = oldLocal }()

	l := loadListing(t)
	dir := t.TempDir()
	rebuildTree(t, dir, l)
	before := snapshot(t, dir)

	confs := loadConfs(t, dir, "")
	reloadConfs := loadConfs(t, t.TempDir(), "")
	now := time.Unix(l.CapturedAt, 0).Add(10 * time.Minute)

	// The recording path of the captured configuration holds no segment, only stray files
	// named after camera ids: a pass must leave every file and directory of every spool alone.
	runPassWithReload(t, dir, confs, reloadConfs, now, before)
}

func TestCleanerRealSpoolRemovesOnlyExpiredSegments(t *testing.T) {
	oldLocal := time.Local
	time.Local = time.UTC
	defer func() { time.Local = oldLocal }()

	l := loadListing(t)

	for _, root := range []string{"tmp/recorder", "tmp/spools/disk2", "tmp/spools/disk3"} {
		t.Run(root, func(t *testing.T) {
			dir := t.TempDir()
			rebuildTree(t, dir, l)
			before := snapshot(t, dir)

			confs := loadConfs(t, dir, root)
			reloadConfs := loadConfs(t, t.TempDir(), root)

			// the captured ages are up to ~4 minutes: with recordDeleteAfter of 5 minutes
			// and this clock, the older flat segments are expired and the newer are not.
			now := time.Unix(l.CapturedAt, 0).Add(150 * time.Second)

			want, removed := expectedAfterPass(before, root, now, confs)
			require.NotZero(t, removed, "the fixture must hold expired segments")

			unexpired := 0
			for rel, typ := range want {
				if typ == "file" && path.Dir(rel) == root && strings.HasSuffix(rel, ".mp4") &&
					flatSegmentRe.MatchString(path.Base(rel)) {
					unexpired++
				}
			}
			require.NotZero(t, unexpired, "the fixture must hold unexpired segments")

			runPassWithReload(t, dir, confs, reloadConfs, now, want)
		})
	}
}
