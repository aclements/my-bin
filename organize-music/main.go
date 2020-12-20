package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/dhowden/tag"
)

func main() {
	albums := make(map[string]string)

	filepath.Walk("austin-google-music-2020/Google Play Music", func(path string, info os.FileInfo, err error) error {
		if info.IsDir() || !strings.HasSuffix(path, ".mp3") {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			log.Println(err)
			return nil
		}
		defer f.Close()

		m, err := tag.ReadFrom(f)
		if err != nil {
			log.Println("reading from ", path, ":", err)
			return nil
		}

		album, artist := m.Album(), m.AlbumArtist()

		if album == "" || artist == "" {
			log.Println("missing metadata for", path)
			return nil
		}

		have, ok := albums[album]
		if !ok {
			albums[album] = artist
		} else if have != artist {
			log.Println("multiple artists for", album)
		}

		dir := filepath.Join("austin-google-music-2020/organized", artist, album)
		err = os.MkdirAll(dir, 0777)
		if err != nil {
			log.Println(err)
			return nil
		}

		target, err := filepath.Rel(dir+"/", path)
		if err != nil {
			log.Println(err)
			return nil
		}
		err = os.Symlink(target, filepath.Join(dir, filepath.Base(path)))
		if err != nil {
			log.Println(err)
			return nil
		}

		return nil
	})
}
