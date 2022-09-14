package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/asdine/cueimports"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(2)
	}
}

func run() error {
	flag.Parse()

	if flag.NArg() == 0 {
		if err := processInput("", os.Stdin, os.Stdout); err != nil {
			return err
		}
	}

	for i := 0; i < flag.NArg(); i++ {
		path := flag.Arg(i)
		switch dir, err := os.Stat(path); {
		case err != nil:
			return err
		case dir.IsDir():
			if err := filepath.Walk(path, func(path string, info fs.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !isCueFile(info) {
					return nil
				}
				return processFile(path)
			}); err != nil {
				return err
			}
		default:
			if err := processFile(path); err != nil {
				return err
			}
		}
	}
	return nil
}

func isCueFile(f os.FileInfo) bool {
	name := f.Name()
	return !f.IsDir() && !strings.HasPrefix(name, ".") && strings.HasSuffix(name, ".cue")
}

func processInput(filename string, r io.Reader, w io.Writer) error {
	content, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	res, err := cueimports.Import(filename, content)
	if err != nil {
		return err
	}

	_, err = w.Write(res)
	return err
}

func processFile(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		return err
	}

	res, err := cueimports.Import(path, content)
	if err != nil {
		return err
	}

	if bytes.Equal(content, res) {
		return nil
	}

	err = f.Truncate(0)
	if err != nil {
		return err
	}

	_, err = f.Seek(0, 0)
	if err != nil {
		return err
	}

	_, err = f.Write(res)
	return err
}
