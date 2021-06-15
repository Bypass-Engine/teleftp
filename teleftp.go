// teleftp experimental development for the FASTPANEL® hosting control panel
// the authors of this software do not encourage you to use it
// you assume all risks for your Telegram account
// disclaimer of any obligations, claims, complaints
package main

import (
	"archive/zip"
	"github.com/goftp/file-driver"
	"github.com/goftp/server"
	"github.com/joho/godotenv"
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

func Archive(source, target string) error {
	file, err := os.Create(target)
	if err != nil {
		return err
	}

	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(file)

	w := zip.NewWriter(file)
	defer func(w *zip.Writer) {
		err := w.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(w)

	walker := func(path string, info os.FileInfo, err error) error {
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

		f, err := w.Create(path)
		if err != nil {
			return err
		}

		if _, err = io.Copy(f, file); err != nil {
			return err
		}

		return nil
	}

	if err = filepath.Walk(source, walker); err != nil {
		return err
	}

	if err = os.RemoveAll(source); err != nil {
		return err
	}

	return nil
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

func checkBackupProc() bool {
	matches, _ := filepath.Glob("/proc/*/exe")
	for _, file := range matches {
		target, _ := os.Readlink(file)
		if len(target) > 0 && strings.Contains(target, "/usr/local/fastpanel2/app/fastbackup") {
			return true
		}
	}

	return false
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

			if f.IsDir() {
				if e, err := IsEmpty(fullPath); err == nil {
					if e {
						return
					}
				} else {
					log.Fatal(err)
				}

				log.Println("↓ |", f.Name())

				if err := Archive(fullPath, fullPath+".zip"); err != nil {
					log.Fatal(err)
				}

				chat, _ := strconv.Atoi(os.Getenv("TELEGRAM_CHAT"))
				if _, err := agent.Send(&tb.Chat{ID: int64(chat)}, &tb.Document{
					File:     tb.FromDisk(fullPath + ".zip"),
					FileName: f.Name() + ".zip",
				}); err != nil {
					log.Fatal(err)
				}

				log.Println("↑ |", f.Name())

				if err := os.RemoveAll(fullPath + ".zip"); err != nil {
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

	c := false
	for {
		n := checkBackupProc()

		if c && !n {
			go filesHandler()
		}

		c = n

		time.Sleep(1 * time.Second)
	}
}
