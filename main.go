package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	std_filepath "path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gen2brain/beeep"
	"github.com/gen2brain/go-unarr"
)

type ArchiveExtractor func(filepath, dir_path string)

func getExtractor(ext string) ArchiveExtractor {
	switch ext {
	case ".zip":
		return unzip
	case ".7z", ".rar":
		return unarrExtract
	case ".tar.gz":
		return untargz
	default:
		return nil
	}
}

func event_listener(watcher *fsnotify.Watcher) {
	wait := 2 * time.Second
	var mutex sync.Mutex
	timers := make(map[string]*time.Timer)

	callback := func(e fsnotify.Event) {
		fmt.Printf("e.Name: %v\n", e)
		process_file(e.Name)
		mutex.Lock()
		delete(timers, e.Name)
		mutex.Unlock()
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			if !event.Has(fsnotify.Create) && !event.Has(fsnotify.Write) {
				continue
			}

			if strings.HasSuffix(event.Name, ".tmp") || strings.HasSuffix(event.Name, ".opdownload") {
				continue
			}

			name := event.Name
			if strings.HasSuffix(event.Name, ".part") {
				parts := strings.SplitN(name, ".", 3)
				if len(parts) != 3 {
					continue
				}
				name = parts[0] + "." + strings.TrimSuffix(parts[2], ".part")
			}

			mutex.Lock()
			t, ok := timers[name]
			mutex.Unlock()

			if !ok {
				t = time.AfterFunc(wait, func() { callback(event) })

				mutex.Lock()
				timers[name] = t
				mutex.Unlock()
			} else {
				t.Reset(wait)
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			fmt.Printf("error: %s\n", err)
		}
	}
}

func unzip(filepath, dir_path string) {
	archive, err := zip.OpenReader(filepath)
	if err != nil {
		panic(err)
	}
	defer archive.Close()

	for _, f := range archive.File {
		fmt.Printf("extracting %s\n", f.Name)
		mode := f.Mode()
		if mode.IsDir() {
			tmp := path.Join(dir_path, f.Name)
			err = os.MkdirAll(tmp, 0750)
			if err != nil {
				panic(err)
			}
		}
		if mode.IsRegular() {
			rc, err := f.Open()
			if err != nil {
				panic(err)
			}
			defer rc.Close()
			tmp := path.Join(dir_path, f.Name)
			err = os.MkdirAll(path.Dir(tmp), 0750)
			if err != nil {
				panic(err)
			}
			file, err := os.Create(tmp)
			if err != nil {
				panic(err)
			}
			defer file.Close()

			_, err = io.Copy(file, rc)
			if err != nil {
				panic(err)
			}
		}
	}
}

func untargz(filepath, dir_path string) {
	archive, err := os.Open(filepath)
	if err != nil {
		panic(err)
	}
	defer archive.Close()

	gz, err := gzip.NewReader(archive)
	if err != nil {
		panic(err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
		switch header.Typeflag {
		case tar.TypeReg:
			tmp := path.Join(dir_path, header.Name)
			file, err := os.Create(tmp)
			if err != nil {
				panic(err)
			}
			defer file.Close()

			_, err = io.Copy(file, reader)
			if err != nil {
				panic(err)
			}
		case tar.TypeDir:
			tmp := path.Join(dir_path, header.Name)
			err = os.MkdirAll(tmp, 0750)
			if err != nil {
				panic(err)
			}
		}
	}

}

func unarrExtract(filepath string, dir_path string) {
	archive, err := unarr.NewArchive(filepath)
	if err != nil {
		panic(err)
	}
	defer archive.Close()
	_, err = archive.Extract(dir_path)
	if err != nil {
		panic(err)
	}
}

func process_file(filepath string) {

	if strings.HasSuffix(filepath, ".tmp") || strings.HasSuffix(filepath, ".opdownload") || strings.HasSuffix(filepath, ".part") {
		return
	}

	stat, err := os.Stat(filepath)
	if err != nil {
		panic(err)
	}

	if stat.IsDir() {
		return
	}

	ext := path.Ext(filepath)
	ext2 := path.Ext(strings.TrimSuffix(filepath, ext))
	if ext2 == ".tar" {
		ext = ext2 + ext
	}
	dir_path := strings.TrimSuffix(filepath, ext)

	archiveExtractor := getExtractor(ext)

	if archiveExtractor == nil {
		return
	}

	err = beeep.Notify("AutoExtractor: ", "extracting "+filepath, "download_icon.png")
	if err != nil {
		panic(err)
	}

	err = os.MkdirAll(dir_path, 0750)
	if err != nil {
		panic(err)
	}
	archiveExtractor(filepath, dir_path)
	tmp := path.Join(dir_path, std_filepath.Base(filepath))
	err = os.Rename(filepath, tmp)
	if err != nil {
		panic(err)
	}
	err = beeep.Notify("AutoExtractor: ", fmt.Sprintf("%s extracted successfully", dir_path), "extracted_icon.png")
	if err != nil {
		panic(err)
	}
}

func main() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		panic(err)
	}
	defer watcher.Close()
	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	downloads := path.Join(homeDir, "Downloads")
	err = watcher.Add(downloads)
	if err != nil {
		panic(err)
	}

	go event_listener(watcher)

	<-make(chan struct{})

}
