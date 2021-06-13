package main

import (
	"archive/zip"
	"github.com/goftp/file-driver"
	"github.com/goftp/server"
	"github.com/joho/godotenv"
	"github.com/sger/go-hashdir"
	tb "gopkg.in/tucnak/telebot.v2"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var agent *tb.Bot

func Zip(source, target string) error {
	f, err := os.Create(target)
	if err != nil {
		log.Fatal(err)
	}

	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(f)

	archive := zip.NewWriter(f)
	defer func(archive *zip.Writer) {
		err := archive.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(archive)

	info, err := os.Stat(source)
	if err != nil {
		return nil
	}

	var baseDir string
	if info.IsDir() {
		baseDir = filepath.Base(source)
	}

	err = filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		if baseDir != "" {
			header.Name = filepath.Join(baseDir, strings.TrimPrefix(path, source))
		}

		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}

		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}

		defer func(file *os.File) {
			err := file.Close()
			if err != nil {
				log.Fatal(err)
			}
		}(file)

		_, err = io.Copy(writer, file)

		return err
	})

	if err != nil {
		return err
	}

	err = os.RemoveAll(source)
	if err != nil {
		log.Fatal(err)
	}

	return err
}

func IsEmpty(name string) (bool, error) {
	f, err := os.Open(name)
	if err != nil {
		return false, err
	}

	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(f)

	if _, err = f.Readdirnames(1); err == io.EOF {
		return true, nil
	}

	return false, err
}

func ftpHandler() {
	factory := &filedriver.FileDriverFactory{
		RootPath: os.Getenv("PATH_FILES"),
		Perm:     server.NewSimplePerm("root", "root"),
	}

	port, _ := strconv.Atoi(os.Getenv("FTP_PORT"))
	opts := &server.ServerOpts{
		Factory: factory,
		Auth: &server.SimpleAuth{
			Name:     os.Getenv("FTP_USER"),
			Password: os.Getenv("FTP_PASS"),
		},
		Hostname: os.Getenv("FTP_HOST"),
		Port:     port,
		Logger:   new(server.DiscardLogger),
	}

	log.Println("FTP is running and listening...")

	if err := server.NewServer(opts).ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func filesHandler() {
	if files, err := ioutil.ReadDir(os.Getenv("PATH_FILES")); err == nil {
		for _, f := range files {
			fullPath := os.Getenv("PATH_FILES") + f.Name()
			hash, _ := hashdir.Create(fullPath, "md5")

			if f.IsDir() {
				log.Println("↓ |", f.Name(), f.Size(), "bytes")

				time.Sleep(5 * time.Minute)

				if e, err := IsEmpty(fullPath); err == nil {
					if e {
						return
					}
				} else {
					log.Fatal(err)
				}

				newHash, _ := hashdir.Create(fullPath, "md5")
				if hash != newHash {
					return
				}

				err := Zip(os.Getenv("PATH_FILES")+f.Name(), os.Getenv("PATH_FILES")+f.Name()+".zip")
				if err != nil {
					log.Fatal(err)
				}
			} else {
				newHash, _ := hashdir.Create(fullPath, "md5")
				if hash != newHash {
					return
				}

				chat, _ := strconv.Atoi(os.Getenv("TELEGRAM_CHAT"))
				_, err := agent.Send(&tb.Chat{ID: int64(chat)}, &tb.Document{
					File:     tb.FromDisk(fullPath),
					FileName: f.Name(),
				})
				if err != nil {
					log.Fatal(err)
				}

				log.Println("↑ |", f.Name(), f.Size(), "bytes")

				if err := os.RemoveAll(fullPath); err != nil {
					log.Fatal(err)
				}
			}
		}
	} else {
		log.Fatal(err)
	}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if _, err := os.Stat(os.Getenv("PATH_FILES")); os.IsNotExist(err) {
		_ = os.Mkdir(os.Getenv("PATH_FILES"), os.ModeDir)
	}

	if err := godotenv.Load(); err != nil {
		log.Fatal("No \".env\" file found!")
	}

	go ftpHandler()

	var err error
	if agent, err = tb.NewBot(tb.Settings{
		URL:    os.Getenv("TELEGRAM_URL"),
		Token:  os.Getenv("TELEGRAM_TOKEN"),
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	}); err != nil {
		log.Fatal(err)
	}

	go agent.Start()

	for {
		filesHandler()

		time.Sleep(1 * time.Second)
	}
}
