package main

import (
	"archive/zip"
	"crypto/md5"
	"encoding/hex"
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

	_, err = f.Readdirnames(1)
	if err == io.EOF {
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
	if _, err := os.Stat(os.Getenv("PATH_FILES")); os.IsNotExist(err) {
		_ = os.Mkdir(os.Getenv("PATH_FILES"), os.ModeDir)
	}

	files, err := ioutil.ReadDir(os.Getenv("PATH_FILES"))
	if err != nil {
		log.Fatal(err)
	}

	for _, f := range files {
		full := os.Getenv("PATH_FILES") + f.Name()
		hash, _ := hashdir.Create(full, "md5")
		//log.Println("hash", hash)

		time.Sleep(30 * time.Minute)

		if f.IsDir() {
			if is, err := IsEmpty(full); err != nil {
				log.Fatal(err)
			} else {
				if is {
					return
				}
			}

			newHash, _ := hashdir.Create(full, "md5")
			//log.Println("newHash DIR", newHash)
			if hash != newHash {
				return
			}

			err := Zip(os.Getenv("PATH_FILES")+f.Name(), os.Getenv("PATH_FILES")+f.Name()+".zip")
			if err != nil {
				log.Fatal(err)
			}
		} else {
			if !Contains(cache, hash) {
				newHash, _ := hashdir.Create(full, "md5")
				//log.Println("newHash File", newHash)
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
