package main

import (
	"crypto/md5"
	"encoding/hex"
	"github.com/goftp/file-driver"
	"github.com/goftp/server"
	"github.com/joho/godotenv"
	tb "gopkg.in/tucnak/telebot.v2"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"time"
)

var (
	cache []string
	agent *tb.Bot
)

func Contains(a []string, x string) bool {
	for _, n := range a {
		if x == n {
			return true
		}
	}

	return false
}

func Delete(a []string, x string) {
	index := 0
	for _, n := range a {
		if x != n {
			a[index] = n
			index++
		}
	}

	cache = a[:index]
}

func Sum(path string) (string, error) {
	var res string
	file, err := os.Open(path)
	if err != nil {
		return res, err
	}

	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(file)

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return res, err
	}

	res = hex.EncodeToString(hash.Sum(nil)[:16])

	return res, nil
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
	if _, err := os.Stat(os.Getenv("PATH_FILES")); os.IsNotExist(err) {
		_ = os.Mkdir(os.Getenv("PATH_FILES"), os.ModeDir)
	}

	files, err := ioutil.ReadDir(os.Getenv("PATH_FILES"))
	if err != nil {
		log.Fatal(err)
	}

	for _, f := range files {
		full := os.Getenv("PATH_FILES") + f.Name()
		hash, _ := Sum(full)

		if !Contains(cache, hash) {
			newHash, _ := Sum(full)
			if hash != newHash {
				return
			}

			cache = append(cache, hash)

			log.Println("↓ |", f.Name(), f.Size(), "bytes")

			chat, _ := strconv.Atoi(os.Getenv("TELEGRAM_CHAT"))
			_, err := agent.Send(&tb.Chat{ID: int64(chat)}, &tb.Document{
				File:     tb.FromDisk(full),
				FileName: f.Name(),
			})
			if err != nil {
				log.Fatal(err)
			}

			log.Println("↑ |", f.Name(), f.Size(), "bytes")

			if err := os.RemoveAll(full); err != nil {
				log.Fatal(err)
			}

			Delete(cache, hash)
		}
	}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

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
