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
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

var (
	agent                                              *tb.Bot
	sysClear                                           bool
	ftpPort, tgChat                                    int64
	ftpUser, ftpPass, ftpHost, tgUrl, tgToken, sysPath string
)

func checkProc() bool {
	if out, err := exec.Command("sh", "-c", "pidof fastbackup").Output(); err == nil && out != nil {
		return true
	}

	return false
}

func ftpHandler() {
	factory := &filedriver.FileDriverFactory{
		RootPath: sysPath,
		Perm:     server.NewSimplePerm("root", "root"),
	}

	opts := &server.ServerOpts{
		Factory: factory,
		Auth: &server.SimpleAuth{
			Name:     ftpUser,
			Password: ftpPass,
		},
		Hostname: ftpHost,
		Port:     int(ftpPort),
		Logger:   new(server.DiscardLogger),
	}

	log.Println("FTP is running and listening...")

	if err := server.NewServer(opts).ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func filesHandler() {
	if files, err := ioutil.ReadDir(sysPath); err == nil {
		for _, f := range files {
			if f.IsDir() {
				fullPath := sysPath + f.Name()
				fullPathZip := fullPath + ".zip"

				log.Println("↓ |", f.Name())

				if err := archiver.Archive([]string{fullPath}, fullPathZip); err != nil {
					log.Fatal(err)
				}

				if _, err := agent.Send(&tb.Chat{ID: tgChat}, &tb.Document{
					File:     tb.FromDisk(fullPathZip),
					FileName: f.Name() + ".zip",
				}); err != nil {
					log.Fatal(err)
				}

				log.Println("↑ |", f.Name())

				if err := os.RemoveAll(fullPath); err != nil {
					log.Fatal(err)
				}

				if sysClear {
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

	if config, err := toml.LoadFile("config.toml"); err == nil {
		ftpUser = config.Get("ftp.user").(string)
		ftpPass = config.Get("ftp.pass").(string)
		ftpHost = config.Get("ftp.host").(string)
		ftpPort = config.Get("ftp.port").(int64)
		tgChat = config.Get("tg.chat").(int64)
		tgUrl = config.Get("tg.url").(string)
		tgToken = config.Get("tg.token").(string)
		sysPath = config.Get("sys.path").(string)
		sysClear = config.Get("sys.clear").(bool)
	} else {
		return err
	}

	if _, err := os.Stat(sysPath); os.IsNotExist(err) {
		_ = os.Mkdir(sysPath, os.ModeDir)
	}

	go ftpHandler()

	if !strings.Contains(tgUrl, "api.telegram.org") {
		if _, err := http.Get("https://api.telegram.org/bot" + tgToken + "/logOut"); err != nil {
			log.Println(err)
		} // https://core.telegram.org/bots/api#logout
	} // If you changed the local server to the official one, you have to wait ~10 minutes after the last (logOut).

	var err error
	if agent, err = tb.NewBot(tb.Settings{
		URL:    tgUrl,
		Token:  tgToken,
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	}); err != nil {
		return err
	}

	go agent.Start()

	if _, err := agent.Send(&tb.Chat{ID: tgChat}, "[teleftp] FTP is running and listening..."); err != nil {
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
