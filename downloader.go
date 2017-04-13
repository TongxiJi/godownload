package main

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

var wg sync.WaitGroup

func hash(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

func _fileSize(file string) (int, error) {
	f, e := os.Stat(file)
	if e != nil {
		return 0, e
	}
	return int(f.Size()), nil
}

func fileSize(url string) int {
	res, _ := http.Head(url)
	maps := res.Header
	length, _ := strconv.Atoi(maps["Content-Length"][0]) // Get the content length from
	return length
}

func _getChunk(url string, min int, max int, tempname string) {

	body := make([]string, 1)

	client := &http.Client{}
	req, _ := http.NewRequest("GET", url, nil)
	range_header := "bytes=" + strconv.Itoa(min) + "-" + strconv.Itoa(max-1) //Add the data for the Range header of the form "bytes=0-100"
	req.Header.Add("Range", range_header)
	resp, _ := client.Do(req)
	defer resp.Body.Close()
	reader, _ := ioutil.ReadAll(resp.Body)
	body[0] = string(reader)
	ioutil.WriteFile(tempname, []byte(string(body[0])), 0x777) // Write
}

func getChunk(url string, min int, max int, tempname string) {
	// check the file exists or not;
	// if exists, check the size;
	if _, err := os.Stat(tempname); os.IsNotExist(err) {

		body := make([]string, 1)

		client := &http.Client{}
		req, _ := http.NewRequest("GET", url, nil)
		range_header := "bytes=" + strconv.Itoa(min) + "-" + strconv.Itoa(max-1) //Add the data for the Range header of the form "bytes=0-100"
		req.Header.Add("Range", range_header)
		resp, _ := client.Do(req)
		defer resp.Body.Close()
		reader, _ := ioutil.ReadAll(resp.Body)
		body[0] = string(reader)
		ioutil.WriteFile(tempname, []byte(string(body[0])), 0x777) // Write

	} else if size, err := _fileSize(tempname); err == nil && size != (max-min) {
		csize := size

		min = min + csize + 1

		f, err := os.OpenFile(tempname, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			panic(err)
		}

		defer f.Close()

		//body := make([]string, 1)

		client := &http.Client{}
		req, _ := http.NewRequest("GET", url, nil)
		range_header := "bytes=" + strconv.Itoa(min) + "-" + strconv.Itoa(max-1) //Add the data for the Range header of the form "bytes=0-100"
		req.Header.Add("Range", range_header)
		resp, _ := client.Do(req)
		defer resp.Body.Close()
		reader, _ := ioutil.ReadAll(resp.Body)
		// body[0] = string(reader)

		if _, err = f.Write(reader); err != nil {
			panic(err)
		}
	}
}

func combineChunks(chunkfiles []string, dest string) {
	df, err := os.Create(dest)
	if err != nil {
		panic(err)
	}
	// close fo on exit and check for its returned error
	defer func() {
		if err := df.Close(); err != nil {
			panic(err)
		}
	}()

	for index := range chunkfiles {
		chunkfile := chunkfiles[index]
		fmt.Print(chunkfile)

		cf, err := os.Open(chunkfile)
		if err != nil {
			fmt.Println(err)
			return
		}
		defer cf.Close()

		data := make([]byte, 4096)
		zeroes := 0
		for {
			data = data[:cap(data)]
			n, err := cf.Read(data)
			if err != nil {
				if err == io.EOF {
					break
				}
				fmt.Println(err)
				return
			}
			data = data[:n]
			for _, b := range data {
				if b == 0 {
					zeroes++
				}
			}

			df.Write(data)
		}

	}
}

func _download(url string, chunks int) {
	length := fileSize(url)
	limit := chunks           // 10 Go-routines for the process so each downloads 18.7MB
	len_sub := length / limit // Bytes for each Go-routine

	folder := strconv.Itoa(int(hash(url)))

	for i := 0; i < limit; i++ {
		wg.Add(1)
		min := len_sub * i       // Min range
		max := len_sub * (i + 1) // Max range
		// fmt.Print(min, max, "\n")
		path := filepath.Join(folder, strconv.Itoa(i))
		go func(url string, min int, max int, tempname string) {
			getChunk(url, min, max, tempname)
			wg.Done()
		}(url, min, max, path)
	}

	path := filepath.Join(folder, strconv.Itoa(-1))
	wg.Add(1)
	go func(url string, min int, max int, tempname string) {
		getChunk(url, min, max, tempname)
		wg.Done()
	}(url, len_sub*limit, length, path)
	wg.Wait()

}

func Download(url string, chunks int, dest string) {
	length := fileSize(url)
	limit := chunks           // Go-routines for the process so each downloads 18.7MB
	len_sub := length / limit // Bytes for each Go-routine
	diff := length % limit    // Get the remaining for the last request

	// fmt.Print(length, limit, len_sub, diff)

	folder := strconv.Itoa(int(hash(url)))

	os.MkdirAll(folder, 0777)

	files := make([]string, limit+1)
	sizes := make([]int, limit+1)

	for i := 0; i < limit; i++ {
		files[i] = filepath.Join(folder, strconv.Itoa(i))
		sizes[i] = len_sub
	}

	files[limit] = filepath.Join(folder, strconv.Itoa(-1))
	sizes[limit] = diff

	flag := false
	for try := 0; try < 10; try++ {
		count := 0
		for index := range files {
			//size, _ := _fileSize(files[index])
			// fmt.Print(sizes[index], size, "\n")
			if size, err := _fileSize(files[index]); err == nil && size == sizes[index] {
				count = count + 1
			} else {
				fmt.Print(files[index], " is missing!", "\n")
			}
		}
		fmt.Print("count: ", count, " / ", chunks+1, "\n")
		if count == chunks+1 {
			fmt.Print("Complete", "\n")
			flag = true
			break
		} else {
			_download(url, chunks)
		}
	}

	if flag {
		combineChunks(files, dest)
	}

}

func main() {
	// Download("http://45.78.19.85:8001/win32-xtensa-lx106-elf-gb404fb9-2.tar.gz", 200)
	byt, _ := ioutil.ReadFile("config.json")
	var dat map[string]interface{}

	if err := json.Unmarshal(byt, &dat); err != nil {
		panic(err)
	}
	url := dat["url"].(string)
	chunks := int(dat["chunks"].(float64))
	dest := dat["dest"].(string)

	fmt.Println("target: ", dat["url"], "\n")
	fmt.Print("Chunks: ", dat["chunks"], "\n")

	length := fileSize(url)
	limit := chunks           // Go-routines for the process so each downloads 18.7MB
	len_sub := length / limit // Bytes for each Go-routine
	diff := length % limit    // Get the remaining for the last request

	fmt.Print("Length: ", length, "\n")
	fmt.Print("Bytes for each subrotine: ", len_sub, "\n")
	fmt.Print("Residual: ", diff, "\n")
	fmt.Print("Total subroutines: ", chunks+1, "\n")
	fmt.Print("temperory folder: ", strconv.Itoa(int(hash(url))), "\n")

	Download(url, chunks, dest)

}
