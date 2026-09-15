package main

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"flpmine/audio"
	"flpmine/classifier"
	"flpmine/flp"
	"flpmine/midiwriter"
)

var drumTags = map[string]bool{
	"kick": true, "808": true, "snare": true, "clap": true, "rim": true,
	"hihat": true, "open_hat": true, "crash": true, "cymbal": true,
	"shaker": true, "perc": true, "fill": true,
}

// fileRow represente une ligne de la liste de statut.
type fileRow struct {
	path   string
	status string
}

var (
	mu       sync.Mutex
	rows     []fileRow
	rowIndex = map[string]int{}

	outputFolder = binding.NewString()
	logText      = binding.NewString()

	fileList *widget.List
	win      fyne.Window
)

func main() {
	a := app.NewWithID("com.lewordstudio.flpmine")
	win = a.NewWindow("FlpMine — Extracteur de sons FL Studio")
	win.Resize(fyne.NewSize(820, 620))

	home, _ := os.UserHomeDir()
	outputFolder.Set(filepath.Join(home, "Desktop", "FlpMine_Export"))

	dropLabel := widget.NewLabel("Glisse tes fichiers .flp ici (plusieurs a la fois possible)")

	chooseOutputBtn := widget.NewButton("Changer le dossier de sortie...", func() {
		d := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			outputFolder.Set(uri.Path())
		}, win)
		d.Show()
	})
	outputLabel := widget.NewLabelWithData(outputFolder)

	scanFolderBtn := widget.NewButton("...ou scanner un dossier de projets", func() {
		d := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			var flpFiles []string
			filepath.WalkDir(uri.Path(), func(path string, de os.DirEntry, err error) error {
				if err == nil && !de.IsDir() && strings.EqualFold(filepath.Ext(path), ".flp") {
					flpFiles = append(flpFiles, path)
				}
				return nil
			})
			if len(flpFiles) == 0 {
				appendLog(fmt.Sprintf("Aucun fichier .flp trouve dans %s\n", uri.Path()))
				return
			}
			go processFiles(flpFiles)
		}, win)
		d.Show()
	})

	openOutputBtn := widget.NewButton("Ouvrir le dossier de sortie", func() {
		folder, _ := outputFolder.Get()
		if folder == "" {
			return
		}
		u := storage.NewFileURI(folder)
		a.OpenURL(u)
	})

	fileList = widget.NewList(
		func() int {
			mu.Lock()
			defer mu.Unlock()
			return len(rows)
		},
		func() fyne.CanvasObject {
			return container.NewHBox(widget.NewLabel("fichier"), widget.NewLabel("statut"))
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			mu.Lock()
			defer mu.Unlock()
			if id < 0 || id >= len(rows) {
				return
			}
			box := obj.(*fyne.Container)
			box.Objects[0].(*widget.Label).SetText(filepath.Base(rows[id].path))
			box.Objects[1].(*widget.Label).SetText(rows[id].status)
		},
	)

	logEntry := widget.NewEntryWithData(logText)
	logEntry.MultiLine = true
	logEntry.Disable()
	logScroll := container.NewScroll(logEntry)

	topBar := container.NewVBox(
		dropLabel,
		container.NewGridWithColumns(2, chooseOutputBtn, outputLabel),
		container.NewGridWithColumns(2, scanFolderBtn, openOutputBtn),
	)

	split := container.NewVSplit(fileList, logScroll)
	split.Offset = 0.55

	content := container.NewBorder(topBar, nil, nil, nil, split)
	win.SetContent(content)

	// Glisser-deposer de fichiers .flp directement sur la fenetre.
	win.SetOnDropped(func(pos fyne.Position, uris []fyne.URI) {
		var flpFiles []string
		for _, u := range uris {
			if strings.EqualFold(filepath.Ext(u.Path()), ".flp") {
				flpFiles = append(flpFiles, u.Path())
			}
		}
		if len(flpFiles) == 0 {
			appendLog("Aucun fichier .flp dans ce que tu as depose.\n")
			return
		}
		go processFiles(flpFiles)
	})

	win.ShowAndRun()
}

func appendLog(text string) {
	cur, _ := logText.Get()
	logText.Set(cur + text)
}

func setStatus(path, status string) {
	mu.Lock()
	idx, ok := rowIndex[path]
	if !ok {
		idx = len(rows)
		rows = append(rows, fileRow{path: path, status: status})
		rowIndex[path] = idx
	} else {
		rows[idx].status = status
	}
	mu.Unlock()
	fileList.Refresh()
}

var sanitizeRe = regexp.MustCompile(`[<>:"/\\|?*]`)

func sanitizeFilename(name string) string {
	name = sanitizeRe.ReplaceAllString(name, "_")
	name = strings.TrimSpace(name)
	if name == "" {
		name = "sans_nom"
	}
	if len(name) > 80 {
		name = name[:80]
	}
	return name
}

func processFiles(paths []string) {
	for _, p := range paths {
		setStatus(p, "En attente...")
	}

	outRoot, _ := outputFolder.Get()
	if err := os.MkdirAll(outRoot, 0755); err != nil {
		appendLog(fmt.Sprintf("[ERREUR] impossible de creer le dossier de sortie : %v\n", err))
		return
	}
	drumkitRoot := filepath.Join(outRoot, "Drumkit")

	totalPatterns := 0
	totalDrumFiles := 0
	mismatchCount := 0

	for _, path := range paths {
		setStatus(path, "Traitement en cours...")
		projectName := sanitizeFilename(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
		appendLog(fmt.Sprintf("\n=== %s ===\n", filepath.Base(path)))

		proj, err := flp.Parse(path)
		if err != nil {
			appendLog(fmt.Sprintf("  [ERREUR parsing] %v\n", err))
			setStatus(path, "Erreur (voir journal)")
			continue
		}

		projectDir := filepath.Join(outRoot, projectName)
		if err := os.MkdirAll(projectDir, 0755); err != nil {
			appendLog(fmt.Sprintf("  [ERREUR] creation dossier projet : %v\n", err))
			setStatus(path, "Erreur (voir journal)")
			continue
		}

		channelsByIdx := make(map[int]*flp.Channel, len(proj.Channels))
		for i := range proj.Channels {
			channelsByIdx[proj.Channels[i].Index] = &proj.Channels[i]
		}

		exported := 0
		for pi, pat := range proj.Patterns {
			if len(pat.Notes) == 0 {
				continue
			}
			chIdxs := pat.ChannelIndexes()
			tag := "unknown"
			var mainChannel *flp.Channel
			if len(chIdxs) > 0 {
				if ch, ok := channelsByIdx[chIdxs[0]]; ok {
					mainChannel = ch
					tag = classifier.ClassifyChannel(*ch)
				}
			}

			patName := pat.Name
			if patName == "" {
				patName = fmt.Sprintf("pattern_%d", pi+1)
			}
			midName := fmt.Sprintf("%02d_%s_%s.mid", pi+1, tag, sanitizeFilename(patName))
			midPath := filepath.Join(projectDir, midName)
			if err := midiwriter.WritePattern(midPath, proj.PPQ, pat.Notes, proj.TempoBPM); err != nil {
				appendLog(fmt.Sprintf("  [ERREUR export MIDI] %s : %v\n", patName, err))
				continue
			}
			exported++

			if mainChannel != nil && mainChannel.SamplePath != "" && drumTags[tag] {
				checkAndCopyToDrumkit(mainChannel, tag, drumkitRoot, &totalDrumFiles, &mismatchCount)
			}
		}

		appendLog(fmt.Sprintf("  -> %d patterns exportes dans %s\n", exported, projectDir))
		setStatus(path, fmt.Sprintf("Extrait (%d patterns)", exported))
		totalPatterns += exported
	}

	appendLog(fmt.Sprintf("\n--- Termine : %d patterns exportes, %d samples classes dans le Drumkit (%d a verifier) ---\n",
		totalPatterns, totalDrumFiles, mismatchCount))
	appendLog(fmt.Sprintf("Tout est dans : %s\n", outRoot))
}

func checkAndCopyToDrumkit(ch *flp.Channel, tag string, drumkitRoot string, totalDrumFiles *int, mismatchCount *int) {
	if _, err := os.Stat(ch.SamplePath); err != nil {
		return
	}

	reason := ""
	mismatch := false
	if strings.EqualFold(filepath.Ext(ch.SamplePath), ".wav") {
		if samples, err := audio.ReadWAV(ch.SamplePath); err == nil {
			feat := audio.Extract(samples)
			result := audio.Verify(tag, feat)
			mismatch = result.Mismatch
			reason = result.Reason
			if mismatch {
				*mismatchCount++
				appendLog(fmt.Sprintf("  [A VERIFIER] %q tague %q — %s\n", ch.Name, tag, reason))
			}
		}
	}

	destDir := filepath.Join(drumkitRoot, tag)
	if mismatch {
		destDir = filepath.Join(drumkitRoot, "_a_verifier", tag)
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return
	}

	destPath := uniqueDestPath(destDir, ch.SampleFilename, ch.SamplePath)
	if destPath == "" {
		return
	}
	if err := copyFile(ch.SamplePath, destPath); err == nil {
		*totalDrumFiles++
	}
}

func uniqueDestPath(dir, filename, srcPath string) string {
	dest := filepath.Join(dir, filename)
	if _, err := os.Stat(dest); err != nil {
		return dest
	}
	if sameContent(srcPath, dest) {
		return ""
	}
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	for i := 2; i < 1000; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s_%d%s", base, i, ext))
		if _, err := os.Stat(candidate); err != nil {
			return candidate
		}
		if sameContent(srcPath, candidate) {
			return ""
		}
	}
	return dest
}

func sameContent(a, b string) bool {
	ha, err1 := fileHash(a)
	hb, err2 := fileHash(b)
	return err1 == nil && err2 == nil && ha == hb
}

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
