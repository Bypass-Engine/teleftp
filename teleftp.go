// teleftp experimental development for the FASTPANEL® hosting control panel
// the authors of this software do not encourage you to use it
// you assume all risks for your Telegram account
// disclaimer of any obligations, claims, complaints
package main

import (
	"github.com/goftp/file-driver"
	"github.com/goftp/server"
	"github.com/mholt/archiver"
	"github.com/pelletier/go-toml"
	tb "gopkg.in/tucnak/telebot.v2"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var (
	agent *tb.Bot
	cfg   = Config{}
)

type FTP struct {
	Port             int
	User, Pass, Host string
}

type Tg struct {
	Chat       int64
	Url, Token string
}

type Sys struct {
	Path  string
	Clear bool
}

type Config struct {
	FTP FTP
	Tg  Tg
	Sys Sys
}

func checkProc() bool {
	if out, err := exec.Command("sh", "-c", "pidof fastbackup").Output(); err == nil && out != nil {
		return true
	}

	return false
}

func ftpHandler() {
	factory := &filedriver.FileDriverFactory{
		RootPath: cfg.Sys.Path,
		Perm:     server.NewSimplePerm("root", "root"),
	}

	opts := &server.ServerOpts{
		Factory: factory,
		Auth: &server.SimpleAuth{
			Name:     cfg.FTP.User,
			Password: cfg.FTP.Pass,
		},
		Hostname: cfg.FTP.Host,
		Port:     cfg.FTP.Port,
		Logger:   new(server.DiscardLogger),
	}

	log.Println("FTP is running and listening...")

	if err := server.NewServer(opts).ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func filesHandler() {
	if files, err := os.ReadDir(cfg.Sys.Path); err == nil {
		for _, f := range files {
			if f.IsDir() {
				fullPath := cfg.Sys.Path + f.Name()
				fullPathZip := fullPath + ".zip"

				log.Println("↓ |", f.Name())

				if err := archiver.Archive([]string{fullPath}, fullPathZip); err != nil {
					log.Fatal(err)
				}

				if _, err := agent.Send(&tb.Chat{ID: cfg.Tg.Chat}, &tb.Document{
					File:     tb.FromDisk(fullPathZip),
					FileName: f.Name() + ".zip",
				}); err != nil {
					log.Fatal(err)
				}

				log.Println("↑ |", f.Name())

				if err := os.RemoveAll(fullPath); err != nil {
					log.Fatal(err)
				}

				if cfg.Sys.Clear {
					if err := os.RemoveAll(fullPathZip); err != nil {
						log.Fatal(err)
					}
				}
			}
		}
	} else {
		log.Fatal(err)
	}
}

func listen() error {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if r, err := os.ReadFile("config.toml"); err == nil {
		_ = toml.Unmarshal(r, &cfg)
	} else {
		return err
	}

	if _, err := os.Stat(cfg.Sys.Path); os.IsNotExist(err) {
		_ = os.Mkdir(cfg.Sys.Path, os.ModeDir)
	}

	go ftpHandler()

	if !strings.Contains(cfg.Tg.Url, "api.telegram.org") {
		if _, err := http.Get("https://api.telegram.org/bot" + cfg.Tg.Token + "/logOut"); err != nil {
			log.Println(err)
		} // https://core.telegram.org/bots/api#logout
	} // If you changed the local server to the official one, you have to wait ~10 minutes after the last (logOut).

	var err error
	if agent, err = tb.NewBot(tb.Settings{
		URL:    cfg.Tg.Url,
		Token:  cfg.Tg.Token,
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	}); err != nil {
		return err
	}

	agent.Handle("/id", func(m *tb.Message) {
		_, _ = agent.Send(m.Chat, strconv.Itoa(int(m.Chat.ID)))
	})

	go agent.Start()

	if _, err := agent.Send(&tb.Chat{ID: cfg.Tg.Chat}, "[teleftp] FTP is running and listening..."); err != nil {
		log.Println(err)
	}

	c := false
	for {
		n := checkProc()

		if c && !n {
			go filesHandler()
		}

		c = n

		time.Sleep(1 * time.Second)
	}
}

func main() {
	if err := listen(); err != nil {
		log.Fatal(err)
	}
}
