package webdav

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"isthmus/internal/logger"
)

type Server struct {
	baseDir string
	prefix  string
	log     *logger.Logger
}

func NewServer(baseDir, prefix string) *Server {
	if prefix == "" {
		prefix = "/webdav"
	}
	return &Server{
		baseDir: baseDir,
		prefix:  strings.TrimSuffix(prefix, "/"),
		log:     logger.WithPrefix("WebDAV"),
	}
}

// MountCommand returns the OS-specific shell command to mount as a virtual drive
func MountCommand(port int, mountPoint string) string {
	webdavURL := fmt.Sprintf("http://127.0.0.1:%d/webdav", port)
	switch runtime.GOOS {
	case "windows":
		if mountPoint == "" {
			mountPoint = "Z:"
		}
		return fmt.Sprintf("New-PSDrive -Name %s -PSProvider FileSystem -Root \\\\127.0.0.1@%d\\DavWWWRoot\\webdav -Persist", strings.TrimSuffix(mountPoint, ":"), port)
	case "darwin":
		if mountPoint == "" {
			mountPoint = "/Volumes/Isthmus"
		}
		return fmt.Sprintf("mkdir -p %s && mount_webdav -i %s %s", mountPoint, webdavURL, mountPoint)
	default: // Linux
		if mountPoint == "" {
			mountPoint = "/mnt/isthmus"
		}
		return fmt.Sprintf("sudo mkdir -p %s && sudo mount -t davfs %s %s", mountPoint, webdavURL, mountPoint)
	}
}

// ServeHTTP handles WebDAV HTTP methods
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, s.prefix)
	relPath = strings.TrimPrefix(relPath, "/")
	fullPath := filepath.Join(s.baseDir, filepath.FromSlash(relPath))

	w.Header().Set("DAV", "1, 2")
	w.Header().Set("MS-Author-Via", "DAV")

	switch r.Method {
	case "OPTIONS":
		w.Header().Set("Allow", "OPTIONS, GET, HEAD, POST, PUT, DELETE, PROPFIND, PROPPATCH, MKCOL, COPY, MOVE")
		w.WriteHeader(http.StatusOK)

	case "PROPFIND":
		s.handlePropfind(w, r, fullPath, relPath)

	case "GET", "HEAD":
		s.handleGet(w, r, fullPath)

	case "PUT":
		s.handlePut(w, r, fullPath)

	case "DELETE":
		s.handleDelete(w, r, fullPath)

	case "MKCOL":
		s.handleMkcol(w, r, fullPath)

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request, fullPath string) {
	fi, err := os.Stat(fullPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if fi.IsDir() {
		http.Error(w, "Directory listing forbidden via raw GET (use PROPFIND)", http.StatusForbidden)
		return
	}

	file, err := os.Open(fullPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	http.ServeContent(w, r, fi.Name(), fi.ModTime(), file)
}

func (s *Server) handlePut(w http.ResponseWriter, r *http.Request, fullPath string) {
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	dst, err := os.Create(fullPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, r.Body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request, fullPath string) {
	if err := os.RemoveAll(fullPath); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMkcol(w http.ResponseWriter, r *http.Request, fullPath string) {
	if err := os.MkdirAll(fullPath, 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handlePropfind(w http.ResponseWriter, r *http.Request, fullPath, relPath string) {
	fi, err := os.Stat(fullPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(207) // Multi-Status

	fmt.Fprintf(w, `<?xml version="1.0" encoding="utf-8"?><D:multistatus xmlns:D="DAV:">`)
	s.writePropstat(w, fi, relPath)

	if fi.IsDir() {
		entries, _ := os.ReadDir(fullPath)
		for _, e := range entries {
			info, err := e.Info()
			if err != nil {
				continue
			}
			childRel := filepath.ToSlash(filepath.Join(relPath, e.Name()))
			s.writePropstat(w, info, childRel)
		}
	}

	fmt.Fprintf(w, `</D:multistatus>`)
}

func (s *Server) writePropstat(w io.Writer, fi os.FileInfo, relPath string) {
	href := s.prefix + "/" + strings.TrimPrefix(relPath, "/")
	if fi.IsDir() && !strings.HasSuffix(href, "/") {
		href += "/"
	}

	resType := ""
	if fi.IsDir() {
		resType = "<D:resourcetype><D:collection/></D:resourcetype>"
	} else {
		resType = "<D:resourcetype/>"
	}

	modTime := fi.ModTime().UTC().Format(http.TimeFormat)

	fmt.Fprintf(w, `<D:response>
<D:href>%s</D:href>
<D:propstat>
<D:prop>
<D:displayname>%s</D:displayname>
%s
<D:getcontentlength>%d</D:getcontentlength>
<D:getlastmodified>%s</D:getlastmodified>
</D:prop>
<D:status>HTTP/1.1 200 OK</D:status>
</D:propstat>
</D:response>`, xmlEscape(href), xmlEscape(fi.Name()), resType, fi.Size(), modTime)
}

func xmlEscape(s string) string {
	var buf strings.Builder
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}
