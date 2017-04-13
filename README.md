# godownload
Implementation of a download in Golang.

This library deals with the scenario that no downloader is available, while the target file is rather big. The "Range" is added in the Headers of the Http request in order to resume broken downloads.
