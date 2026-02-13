## httpranger  

A reader implementation backed by HTTP range requests.

---
### Installation  

```bash
go get github.com/burnedskies/httpranger
```

---  

### Example Usage  

```go
package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/burnedskies/httpranger"
)

func main() {
    url := "https://example.com/file.bin"
	r, err := httpranger.NewReader(url)
	if err != nil {
		log.Fatalf("failed to create reader: %v", err)
	}

	// Seek to offset 1 MiB
	_, err = r.Seek(1<<20, io.SeekStart)
	if err != nil {
		log.Fatalf("seek error: %v", err)
	}

	// Read the next 256 KB
	buf := make([]byte, 256<<10)
	n, err := r.Read(buf)
	if err != nil && err != io.EOF {
		log.Fatalf("read error: %v", err)
	}
	fmt.Printf("read %d bytes\n", n)

	// Random access with ReadAt
	off := int64(5 << 20) // 5 MiB
	n, err = r.ReadAt(buf, off)
	if err != nil && err != io.EOF {
		log.Fatalf("readat error: %v", err)
	}
	fmt.Printf("read %d bytes from offset %d\n", n, off)
}
```