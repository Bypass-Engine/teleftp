// teleftp experimental development for the FASTPANEL® hosting control panel
// the authors of this software do not encourage you to use it
// you assume all risks for your Telegram account
// disclaimer of any obligations, claims, complaints
package main

import (
	"github.com/goftp/file-driver"
	"github.com/goftp/server"
	"github.com/joho/godotenv"
	"github.com/mholt/archiver"
	tb "gopkg.in/tucnak/telebot.v2"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strconv"
	"time"
)

var agent *tb.Bot

func checkBackupProc() bool {
	if out, err := exec.Command("sh", "-c", "pidof fastbackup").Output(); err == nil && out != nil {
		return true
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
			if f.IsDir() {
				fullPath := os.Getenv("PATH_FILES") + f.Name()
				fullPathZip := fullPath + ".zip"

				log.Println("↓ |", f.Name())

				if err := archiver.Archive([]string{fullPath}, fullPathZip); err != nil {
					log.Fatal(err)
				}

				chat, _ := strconv.Atoi(os.Getenv("TELEGRAM_CHAT"))
				if _, err := agent.Send(&tb.Chat{ID: int64(chat)}, &tb.Document{
					File:     tb.FromDisk(fullPathZip),
					FileName: f.Name() + ".zip",
				}); err != nil {
					log.Fatal(err)
				}

				log.Println("↑ |", f.Name())

				if err := os.RemoveAll(fullPath); err != nil {
					log.Fatal(err)
				}

				if err := os.RemoveAll(fullPathZip); err != nil {
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

	go func() {
		agent.Start()

		m, _ := strconv.Atoi(os.Getenv("TELEGRAM_CHAT"))
		if _, err := agent.Send(&tb.Chat{ID: int64(m)}, "[teleftp] FTP is running and listening..."); err != nil {
			log.Fatal(err)
		}
	}()

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
