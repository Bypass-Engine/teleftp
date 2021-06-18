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
	"os"
	"os/exec"
	"time"
)

var (
	ftpUser, ftpPass, ftpHost, tgUrl, tgToken string
	ftpPort, tgChat                           int64
	sysPath                                   string
	agent                                     *tb.Bot
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

	if config, err := toml.LoadFile("config.toml"); err == nil {
		ftpUser = config.Get("ftp.user").(string)
		ftpPass = config.Get("ftp.pass").(string)
		ftpHost = config.Get("ftp.host").(string)
		ftpPort = config.Get("ftp.port").(int64)
		tgChat = config.Get("tg.chat").(int64)
		tgUrl = config.Get("tg.url").(string)
		tgToken = config.Get("tg.token").(string)
		sysPath = config.Get("sys.path").(string)
	} else {
		log.Fatal(err)
	}

	if _, err := os.Stat(sysPath); os.IsNotExist(err) {
		_ = os.Mkdir(sysPath, os.ModeDir)
	}

	go ftpHandler()

	var err error
	if agent, err = tb.NewBot(tb.Settings{
		URL:    tgUrl,
		Token:  tgToken,
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	}); err != nil {
		log.Fatal(err)
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
