package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// ─── Config ───────────────────────────────────────────────────────────────────

const (
	permanentDir = "./uploads/permanent"
	partyDir     = "./uploads/party"
	dataDir      = "./data"
	stickersFile = "./data/stickers.json"
	commentsFile = "./data/comments.json"
)

var allowedExts = []string{".jpg", ".jpeg", ".png", ".gif", ".webp"}

var (
	stickersMu sync.Mutex
	commentsMu sync.Mutex
)

// ─── Types ────────────────────────────────────────────────────────────────────

type Photo struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type Sticker struct {
	ID      string  `json:"id"`
	PageID  string  `json:"pageId"`
	Section string  `json:"section"`
	Emoji   string  `json:"emoji"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
}

type Comment struct {
	ID     string `json:"id"`
	PageID string `json:"pageId"`
	Name   string `json:"name"`
	Text   string `json:"text"`
	Date   string `json:"date"`
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	for _, dir := range []string{permanentDir, partyDir, dataDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatal("Erro ao criar pasta:", err)
		}
	}
	initJSON(stickersFile)
	initJSON(commentsFile)

	mux := http.NewServeMux()

	// Pages
	mux.HandleFunc("GET /", handleIndex)

	// Photo APIs
	mux.HandleFunc("GET /api/permanent", listPhotos(permanentDir, "permanent"))
	mux.HandleFunc("GET /api/party", listPhotos(partyDir, "party"))
	mux.HandleFunc("POST /api/party/upload", uploadPhotos(partyDir))

	// Stickers API
	mux.HandleFunc("GET /api/stickers", getStickers)
	mux.HandleFunc("POST /api/stickers", addSticker)
	mux.HandleFunc("DELETE /api/stickers/{id}", deleteSticker)

	// Comments API
	mux.HandleFunc("GET /api/comments", getComments)
	mux.HandleFunc("POST /api/comments", addComment)

	// Static files
	mux.HandleFunc("GET /uploads/permanent/{file}", serveFile(permanentDir))
	mux.HandleFunc("GET /uploads/party/{file}", serveFile(partyDir))
	mux.HandleFunc("GET /download/{section}/{file}", downloadFile)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("🎂 Álbum da Dona Nenê em http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func initJSON(path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		os.WriteFile(path, []byte("[]"), 0644)
	}
}

func sanitize(name string) string {
	name = filepath.Base(name)
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

func handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, indexHTML)
}

func listPhotos(dir, section string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			jsonOK(w, []Photo{})
			return
		}
		photos := []Photo{}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if !slices.Contains(allowedExts, ext) {
				continue
			}
			photos = append(photos, Photo{
				Name: e.Name(),
				URL:  "/uploads/" + section + "/" + e.Name(),
			})
		}
		jsonOK(w, photos)
	}
}

func uploadPhotos(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 50<<20)
		if err := r.ParseMultipartForm(50 << 20); err != nil {
			http.Error(w, "Arquivo muito grande (max 50MB)", http.StatusBadRequest)
			return
		}
		count := 0
		for _, fh := range r.MultipartForm.File["images"] {
			ext := strings.ToLower(filepath.Ext(fh.Filename))
			if !slices.Contains(allowedExts, ext) {
				continue
			}
			src, err := fh.Open()
			if err != nil {
				continue
			}
			name := fmt.Sprintf("%d_%s", time.Now().UnixNano(), sanitize(fh.Filename))
			dst, err := os.Create(filepath.Join(dir, name))
			if err != nil {
				src.Close()
				continue
			}
			io.Copy(dst, src)
			src.Close()
			dst.Close()
			count++
		}
		jsonOK(w, map[string]int{"uploaded": count})
	}
}

func getStickers(w http.ResponseWriter, r *http.Request) {
	stickersMu.Lock()
	defer stickersMu.Unlock()
	data, _ := os.ReadFile(stickersFile)
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func addSticker(w http.ResponseWriter, r *http.Request) {
	var s Sticker
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, "JSON inválido", http.StatusBadRequest)
		return
	}
	s.ID = fmt.Sprintf("s%d", time.Now().UnixNano())

	stickersMu.Lock()
	defer stickersMu.Unlock()
	var list []Sticker
	data, _ := os.ReadFile(stickersFile)
	json.Unmarshal(data, &list)
	list = append(list, s)
	out, _ := json.Marshal(list)
	os.WriteFile(stickersFile, out, 0644)

	jsonOK(w, s)
}

func deleteSticker(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	stickersMu.Lock()
	defer stickersMu.Unlock()
	var list []Sticker
	data, _ := os.ReadFile(stickersFile)
	json.Unmarshal(data, &list)
	filtered := []Sticker{}
	for _, s := range list {
		if s.ID != id {
			filtered = append(filtered, s)
		}
	}
	out, _ := json.Marshal(filtered)
	os.WriteFile(stickersFile, out, 0644)
	w.WriteHeader(http.StatusNoContent)
}

func getComments(w http.ResponseWriter, r *http.Request) {
	commentsMu.Lock()
	defer commentsMu.Unlock()
	data, _ := os.ReadFile(commentsFile)
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func addComment(w http.ResponseWriter, r *http.Request) {
	var c Comment
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, "JSON inválido", http.StatusBadRequest)
		return
	}
	c.ID = fmt.Sprintf("c%d", time.Now().UnixNano())
	c.Date = time.Now().Format("02/01/2006")

	commentsMu.Lock()
	defer commentsMu.Unlock()
	var list []Comment
	data, _ := os.ReadFile(commentsFile)
	json.Unmarshal(data, &list)
	list = append(list, c)
	out, _ := json.Marshal(list)
	os.WriteFile(commentsFile, out, 0644)

	jsonOK(w, c)
}

func serveFile(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filename := filepath.Base(r.PathValue("file"))
		path := filepath.Join(dir, filename)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		
		// Define Content-Type explícito para GIFs
		if strings.HasSuffix(strings.ToLower(filename), ".gif") {
			w.Header().Set("Content-Type", "image/gif")
		}
		
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeFile(w, r, path)
	}
}

func downloadFile(w http.ResponseWriter, r *http.Request) {
	section := r.PathValue("section")
	filename := filepath.Base(r.PathValue("file"))
	dir := partyDir
	if section == "permanent" {
		dir = permanentDir
	}
	path := filepath.Join(dir, filename)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	http.ServeFile(w, r, path)
}

// ─── HTML ─────────────────────────────────────────────────────────────────────

const indexHTML = `<!DOCTYPE html>
<html lang="pt-BR">
<head>
<meta charset="UTF-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1.0"/>
<title>Álbum da Dona Nenê 🌸</title>
<link href="https://fonts.googleapis.com/css2?family=Caveat:wght@400;500;600;700&family=Lora:ital,wght@0,400;0,600;1,400&display=swap" rel="stylesheet">
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}

:root{
  --kraft:#b8924e;
  --kraft-dark:#8a6a30;
  --kraft-light:#dcc080;
  --kraft-bg:#c4a060;
  --cream:#fdf7ee;
  --cream2:#f5ebda;
  --rose:#d4786e;
  --rose-dark:#b05248;
  --teal:#5fa898;
  --teal-light:#80c4b4;
  --lavender:#9488c0;
  --gold:#c89830;
  --gold-light:#e8c060;
  --mint:#70b888;
  --coral:#d88060;
  --yellow:#d8b840;
  --strip1:#d4786e;
  --strip2:#5fa898;
  --strip3:#9488c0;
  --strip4:#c89830;
  --strip5:#70b888;
  --strip6:#d88060;
  --text:#3a2810;
  --text-mid:#6a4820;
  --text-light:#9a7840;
}

body{
  background-color:var(--kraft-bg);
  background-image:
    repeating-linear-gradient(0deg,transparent,transparent 28px,rgba(0,0,0,0.04) 28px,rgba(0,0,0,0.04) 29px),
    repeating-linear-gradient(90deg,transparent,transparent 28px,rgba(0,0,0,0.03) 28px,rgba(0,0,0,0.03) 29px);
  min-height:100vh;
  font-family:'Lora',serif;
  color:var(--text);
}

/* ── COVER ──────────────────────────────────────────────── */
.cover{
  position:relative;
  min-height:100vh;
  display:flex;
  align-items:center;
  justify-content:center;
  overflow:hidden;
  padding:2rem;
}

/* patchwork background panels */
.cover-bg{
  position:absolute;
  inset:0;
  display:grid;
  grid-template-columns:repeat(5,1fr);
  grid-template-rows:repeat(4,1fr);
  gap:0;
  z-index:0;
}
.cover-patch{opacity:.85;}
.cover-patch:nth-child(20n+1){background:#e8c4b8;}
.cover-patch:nth-child(20n+2){background:#c4d8c8;}
.cover-patch:nth-child(20n+3){background:#d8c8e8;}
.cover-patch:nth-child(20n+4){background:#e8d8c0;}
.cover-patch:nth-child(20n+5){background:#c8dce8;}
.cover-patch:nth-child(20n+6){background:#e8c8d0;}
.cover-patch:nth-child(20n+7){background:#d0e8d0;}
.cover-patch:nth-child(20n+8){background:#e8e0c0;}
.cover-patch:nth-child(20n+9){background:#c8c8e8;}
.cover-patch:nth-child(20n+10){background:#e0c8e0;}
.cover-patch:nth-child(20n+11){background:#d8e0c8;}
.cover-patch:nth-child(20n+12){background:#f0d8c0;}
.cover-patch:nth-child(20n+13){background:#c8e0d8;}
.cover-patch:nth-child(20n+14){background:#e8c8c0;}
.cover-patch:nth-child(20n+15){background:#d8c8d8;}
.cover-patch:nth-child(20n+16){background:#e0d8c0;}
.cover-patch:nth-child(20n+17){background:#c0d8e0;}
.cover-patch:nth-child(20n+18){background:#e0c8d8;}
.cover-patch:nth-child(20n+19){background:#d0e0c8;}
.cover-patch:nth-child(20n+20){background:#e8d0c8;}

/* patch borders (scrapbook paper seams) */
.cover-patch{
  border:1px solid rgba(180,140,100,0.3);
  background-clip:padding-box;
}

/* cover border frame */
.cover::after{
  content:'';
  position:absolute;
  inset:12px;
  border:3px solid rgba(180,140,100,0.5);
  border-radius:4px;
  pointer-events:none;
  z-index:2;
}

/* ── cover buttons ── */
.cover-buttons{
  position:absolute;
  top:1.5rem;
  right:1.8rem;
  display:flex;
  gap:.7rem;
  z-index:10;
}
.cover-btn{
  display:flex;
  align-items:center;
  gap:.4rem;
  padding:.55rem 1.1rem;
  border:none;
  border-radius:50px;
  font-family:'Caveat',cursive;
  font-size:1rem;
  font-weight:700;
  cursor:pointer;
  transition:all .25s ease;
  box-shadow:0 3px 12px rgba(0,0,0,.2),0 1px 3px rgba(0,0,0,.1);
  letter-spacing:.02em;
}
.btn-add-photos{background:var(--rose-dark);color:#fff;}
.btn-add-stickers{background:var(--gold);color:#fff;}
.cover-btn:hover{transform:translateY(-2px) rotate(-1.5deg);box-shadow:0 6px 20px rgba(0,0,0,.25);}

/* ── cover flowers ── */
.flower{
  position:absolute;
  pointer-events:none;
  z-index:3;
  animation:floatFlower 3.5s ease-in-out infinite;
  filter:drop-shadow(1px 2px 3px rgba(0,0,0,.15));
}
@keyframes floatFlower{
  0%,100%{transform:translateY(0) rotate(0deg);}
  50%{transform:translateY(-10px) rotate(6deg);}
}

/* ── cover card ── */
.cover-card{
  position:relative;
  z-index:5;
  background:rgba(253,247,238,.92);
  backdrop-filter:blur(6px);
  border-radius:12px;
  padding:3rem 4rem;
  text-align:center;
  box-shadow:0 20px 60px rgba(80,50,20,.25),0 4px 15px rgba(0,0,0,.1);
  border:2px solid rgba(200,152,48,.3);
  max-width:600px;
  width:100%;
}

/* tape strips on card */
.card-tape{
  position:absolute;
  height:22px;
  border-radius:3px;
}
.tape-gold{
  background:repeating-linear-gradient(90deg,rgba(200,152,48,.5) 0,rgba(232,192,96,.5) 8px,rgba(200,152,48,.5) 8px);
}
.tape-teal{
  background:repeating-linear-gradient(90deg,rgba(95,168,152,.5) 0,rgba(128,196,180,.5) 8px,rgba(95,168,152,.5) 8px);
}
.tape-rose{
  background:repeating-linear-gradient(90deg,rgba(212,120,110,.5) 0,rgba(232,160,150,.5) 8px,rgba(212,120,110,.5) 8px);
}

.cover-deco{
  font-size:1.2rem;
  letter-spacing:.6rem;
  color:var(--rose);
  display:block;
  margin-bottom:.5rem;
  opacity:.8;
}
.cover-title{
  font-family:'Caveat',cursive;
  font-size:clamp(2.4rem,6vw,4rem);
  font-weight:700;
  color:var(--rose-dark);
  line-height:1.05;
  text-shadow:2px 3px 0 rgba(255,255,255,.7);
}
.cover-sub{
  font-family:'Lora',serif;
  font-style:italic;
  font-size:1rem;
  color:var(--text-mid);
  margin:.6rem 0 1.5rem;
  letter-spacing:.04em;
}
.cover-badge{
  display:inline-block;
  background:linear-gradient(135deg,var(--rose-dark),var(--rose));
  color:#fff;
  font-family:'Caveat',cursive;
  font-size:1.5rem;
  font-weight:700;
  padding:.5rem 2.2rem;
  border-radius:50px;
  transform:rotate(-2deg);
  box-shadow:0 4px 16px rgba(176,82,72,.4);
  margin-top:.5rem;
}

/* ── ALBUM BODY ─────────────────────────────────────────── */
.album{
  max-width:920px;
  margin:0 auto;
  padding:2rem 1rem 5rem;
}

.section-banner{
  display:flex;
  align-items:center;
  gap:1rem;
  margin:3rem 0 1.5rem;
  position:relative;
}
.section-banner::before,.section-banner::after{
  content:'';
  flex:1;
  height:3px;
  background:linear-gradient(90deg,transparent,rgba(255,255,255,.6),transparent);
  border-radius:2px;
}
.section-banner-text{
  font-family:'Caveat',cursive;
  font-size:1.8rem;
  font-weight:700;
  color:#fff;
  text-shadow:1px 2px 6px rgba(0,0,0,.3);
  white-space:nowrap;
  padding:.3rem .8rem;
  background:rgba(0,0,0,.12);
  border-radius:50px;
  backdrop-filter:blur(4px);
}

/* ── PAGE SPREAD ─────────────────────────────────────────── */
.spread{
  display:flex;
  background:var(--cream);
  border-radius:3px 14px 14px 3px;
  box-shadow:
    0 12px 45px rgba(80,50,10,.3),
    0 3px 10px rgba(0,0,0,.15),
    inset 0 0 0 1px rgba(180,140,80,.15);
  margin-bottom:3.5rem;
  overflow:visible;
  position:relative;
  transition:transform .3s ease,box-shadow .3s ease;
}
.spread:hover{
  transform:translateY(-3px);
  box-shadow:0 20px 60px rgba(80,50,10,.35),0 5px 15px rgba(0,0,0,.2);
}

/* ── BINDING ─────────────────────────────────────────────── */
.binding{
  width:52px;
  flex-shrink:0;
  display:flex;
  flex-direction:column;
  position:relative;
  border-radius:3px 0 0 3px;
  overflow:hidden;
  box-shadow:inset -4px 0 8px rgba(0,0,0,.15);
}
.b-strip{flex:1;min-height:16px;}
.b-strip:nth-child(6n+1){background:var(--strip1);}
.b-strip:nth-child(6n+2){background:var(--strip2);}
.b-strip:nth-child(6n+3){background:var(--strip3);}
.b-strip:nth-child(6n+4){background:var(--strip4);}
.b-strip:nth-child(6n+5){background:var(--strip5);}
.b-strip:nth-child(6n+6){background:var(--strip6);}

/* rings (argolas) */
.b-ring{
  position:absolute;
  left:50%;
  transform:translate(-50%,-50%);
  width:26px;height:26px;
  border-radius:50%;
  background:radial-gradient(circle at 35% 35%,#e8e0d0,#c0b098);
  border:3px solid rgba(255,255,255,.9);
  box-shadow:
    0 2px 6px rgba(0,0,0,.3),
    inset 0 1px 3px rgba(0,0,0,.2);
  z-index:5;
}
/* inner hole */
.b-ring::after{
  content:'';
  position:absolute;
  inset:4px;
  border-radius:50%;
  background:rgba(140,110,70,.4);
  box-shadow:inset 0 1px 3px rgba(0,0,0,.3);
}

/* ── PAGE CONTENT ────────────────────────────────────────── */
.page{
  flex:1;
  padding:2rem 2.2rem 1.8rem;
  background:
    radial-gradient(ellipse at 95% 5%,rgba(200,152,48,.05) 0%,transparent 50%),
    radial-gradient(ellipse at 5% 95%,rgba(212,120,110,.05) 0%,transparent 50%),
    linear-gradient(165deg,var(--cream) 0%,var(--cream2) 100%);
  position:relative;
  border-radius:0 14px 14px 0;
}

/* subtle paper lines */
.page::before{
  content:'';
  position:absolute;
  inset:0;
  background:repeating-linear-gradient(
    180deg,
    transparent,
    transparent 31px,
    rgba(180,150,100,.08) 31px,
    rgba(180,150,100,.08) 32px
  );
  pointer-events:none;
  border-radius:0 14px 14px 0;
}

/* ── PHOTOS ROW ──────────────────────────────────────────── */
.photos-row{
  display:grid;
  grid-template-columns:1fr 1fr;
  gap:2rem;
  position:relative;
  z-index:1;
}

/* ── PHOTO SLOT ──────────────────────────────────────────── */
.photo-slot{position:relative;}
.photo-slot:nth-child(odd){
  transform:rotate(-2deg);
  margin-top:.5rem;
}
.photo-slot:nth-child(even){
  transform:rotate(1.5deg);
  margin-top:1.5rem;
}

.photo-frame{
  background:#fff;
  padding:7px 7px 26px;
  box-shadow:
    2px 3px 10px rgba(0,0,0,.18),
    0 1px 4px rgba(0,0,0,.1);
  position:relative;
  cursor:zoom-in;
  transition:box-shadow .25s;
}
.photo-frame:hover{
  box-shadow:4px 6px 18px rgba(0,0,0,.28),0 2px 6px rgba(0,0,0,.15);
}
.photo-frame img{
  width:100%;
  display:block;
  max-height:260px;
  object-fit:contain;
  background:#f5f0e8;
}
/* GIFs e imagens altas ficam sem corte */
.photo-frame img[src*=".gif"],
.photo-frame img[src*=".GIF"] {
    object-fit: contain;
    background: #f0ece4;
}

/* washi tape on top of frame */
.tape{
  position:absolute;
  height:20px;
  border-radius:2px;
  z-index:3;
  top:-10px;
  left:50%;
}
.tape-a{
  width:60px;
  transform:translateX(-50%) rotate(-4deg);
  background:repeating-linear-gradient(90deg,rgba(200,152,48,.5) 0,rgba(232,192,96,.55) 7px,rgba(200,152,48,.5) 7px);
}
.tape-b{
  width:55px;
  transform:translateX(-50%) rotate(3deg);
  background:repeating-linear-gradient(90deg,rgba(95,168,152,.5) 0,rgba(128,196,180,.55) 7px,rgba(95,168,152,.5) 7px);
}
.tape-c{
  width:65px;
  transform:translateX(-50%) rotate(-2deg);
  background:repeating-linear-gradient(90deg,rgba(212,120,110,.5) 0,rgba(240,160,150,.55) 7px,rgba(212,120,110,.5) 7px);
}

/* photo corners */
.corner{
  position:absolute;
  width:18px;height:18px;
  z-index:4;
}
.corner::before,.corner::after{
  content:'';
  position:absolute;
  background:linear-gradient(135deg,#d4a840,#e8c860);
  border-radius:1px;
}
.corner::before{width:100%;height:3px;top:0;left:0;}
.corner::after{width:3px;height:100%;top:0;left:0;}
.ctlY{top:0;left:0;}
.ctr{top:0;right:0;transform:scaleX(-1);}
.cbl{bottom:26px;left:0;transform:scaleY(-1);}
.cbr{bottom:26px;right:0;transform:scale(-1);}

/* download button on frame */
.photo-dl{
  position:absolute;
  bottom:4px;right:6px;
  font-family:'Caveat',cursive;
  font-size:.75rem;
  color:var(--text-light);
  text-decoration:none;
  opacity:.7;
  transition:opacity .2s;
}
.photo-dl:hover{opacity:1;}

/* ── STICKERS LAYER ──────────────────────────────────────── */
.stickers-layer{
  position:absolute;
  inset:0;
  pointer-events:none;
  z-index:10;
}
.sticker-el{
  position:absolute;
  font-size:2rem;
  line-height:1;
  pointer-events:all;
  cursor:default;
  user-select:none;
  filter:drop-shadow(0 2px 4px rgba(0,0,0,.2));
  transition:transform .15s ease;
}
.sticker-el:hover{
  filter:drop-shadow(0 3px 6px rgba(0,0,0,.3));
  z-index:20;
}

/* ── sticker mode visual ── */
.spread.sticker-mode{
  cursor:crosshair;
  outline:3px dashed var(--gold);
  outline-offset:3px;
}
.spread.sticker-mode .photo-frame{cursor:crosshair;}

/* ── COMMENTS ────────────────────────────────────────────── */
.comments-area{
  position:relative;
  z-index:1;
  margin-top:1.5rem;
  padding-top:1.2rem;
  border-top:2px dashed rgba(184,146,78,.35);
}
.comments-title{
  font-family:'Caveat',cursive;
  font-size:1.25rem;
  font-weight:700;
  color:var(--kraft-dark);
  margin-bottom:.8rem;
}
.comment-list{display:flex;flex-direction:column;gap:.6rem;}
.comment-card{
  background:linear-gradient(135deg,rgba(200,152,48,.08),rgba(200,152,48,.04));
  border-left:3px solid var(--gold-light);
  border-radius:0 8px 8px 0;
  padding:.6rem .9rem;
}
.comment-header{display:flex;align-items:baseline;gap:.5rem;flex-wrap:wrap;}
.comment-name{
  font-family:'Caveat',cursive;
  font-size:1.1rem;
  font-weight:700;
  color:var(--rose-dark);
}
.comment-date{
  font-size:.75rem;
  color:var(--text-light);
  font-style:italic;
}
.comment-body{
  font-size:.9rem;
  color:var(--text);
  font-style:italic;
  margin-top:.25rem;
  line-height:1.5;
}
.comment-body::before{content:'\201C';}
.comment-body::after{content:'\201D';}

/* new comment button */
.btn-new-comment{
  display:inline-flex;
  align-items:center;
  gap:.4rem;
  margin-top:.8rem;
  background:none;
  border:2px solid var(--kraft-light);
  color:var(--kraft-dark);
  padding:.35rem 1rem;
  border-radius:50px;
  font-family:'Caveat',cursive;
  font-size:1rem;
  font-weight:600;
  cursor:pointer;
  transition:all .2s;
}
.btn-new-comment:hover{background:var(--kraft);border-color:var(--kraft);color:#fff;}

.comment-form{
  display:none;
  flex-direction:column;
  gap:.5rem;
  margin-top:.7rem;
  padding:.8rem;
  background:rgba(253,247,238,.8);
  border-radius:10px;
  border:1px solid var(--kraft-light);
}
.comment-form.open{display:flex;}
.cf-input,.cf-textarea{
  width:100%;
  padding:.45rem .8rem;
  border:2px solid var(--kraft-light);
  border-radius:8px;
  font-family:'Caveat',cursive;
  font-size:1rem;
  background:#fff;
  color:var(--text);
  outline:none;
  transition:border-color .2s;
}
.cf-input:focus,.cf-textarea:focus{border-color:var(--gold);}
.cf-textarea{resize:none;height:68px;}
.cf-submit{
  align-self:flex-start;
  background:var(--rose-dark);
  color:#fff;
  border:none;
  padding:.4rem 1.3rem;
  border-radius:50px;
  font-family:'Caveat',cursive;
  font-size:1rem;
  font-weight:700;
  cursor:pointer;
  transition:all .2s;
}
.cf-submit:hover{filter:brightness(1.1);transform:translateY(-1px);}

/* ── PAGINATION NAV ──────────────────────────────────────── */
.pag-wrapper{position:relative;}

/* hide all spreads by default, JS shows the active one */
.pag-wrapper .spread{display:none;}
.pag-wrapper .spread.active{display:flex;}
.pag-wrapper .empty-spread{display:flex;}

.pag-nav{
  display:flex;
  align-items:center;
  justify-content:center;
  gap:1.2rem;
  margin-top:1.2rem;
  margin-bottom:.4rem;
}
.pag-arrow{
  width:44px;height:44px;
  border-radius:50%;
  border:2px solid var(--kraft-light);
  background:var(--cream);
  color:var(--kraft-dark);
  font-size:1.4rem;
  cursor:pointer;
  display:flex;align-items:center;justify-content:center;
  transition:all .2s ease;
  box-shadow:0 2px 8px rgba(0,0,0,.12);
  font-family:'Caveat',cursive;
  font-weight:700;
  line-height:1;
}
.pag-arrow:hover:not(:disabled){
  background:var(--kraft);
  border-color:var(--kraft);
  color:#fff;
  transform:scale(1.1);
  box-shadow:0 4px 14px rgba(0,0,0,.2);
}
.pag-arrow:disabled{opacity:.3;cursor:default;}
.pag-label{
  font-family:'Caveat',cursive;
  font-size:1.15rem;
  color:rgba(255,255,255,.85);
  text-shadow:0 1px 4px rgba(0,0,0,.3);
  min-width:80px;
  text-align:center;
}

/* ── EMPTY STATE ─────────────────────────────────────────── */
.empty-spread{
  display:flex;
  background:var(--cream);
  border-radius:3px 14px 14px 3px;
  box-shadow:0 8px 30px rgba(80,50,10,.2);
  margin-bottom:3rem;
  overflow:hidden;
  min-height:280px;
}
.empty-page{
  flex:1;
  display:flex;
  flex-direction:column;
  align-items:center;
  justify-content:center;
  gap:.8rem;
  padding:2.5rem;
  font-family:'Caveat',cursive;
  color:var(--kraft);
}
.empty-page span{font-size:3.5rem;opacity:.5;}
.empty-page p{font-size:1.2rem;text-align:center;max-width:280px;line-height:1.4;}

/* ── STICKER PICKER ─────────────────────────────────────── */
.sticker-picker{
  position:fixed;
  bottom:2rem;
  left:50%;
  transform:translateX(-50%);
  background:var(--cream);
  border-radius:18px;
  padding:1.1rem 1.5rem 1rem;
  box-shadow:0 15px 50px rgba(0,0,0,.35),0 4px 15px rgba(0,0,0,.15);
  z-index:500;
  display:none;
  flex-direction:column;
  align-items:center;
  gap:.7rem;
  max-width:92vw;
  border:2px solid var(--kraft-light);
}
.sticker-picker.open{display:flex;}
.picker-label{
  font-family:'Caveat',cursive;
  font-size:1.1rem;
  font-weight:700;
  color:var(--kraft-dark);
}
.picker-hint{
  font-family:'Caveat',cursive;
  font-size:.95rem;
  color:var(--teal);
  font-weight:600;
  text-align:center;
}
.sticker-grid{
  display:flex;
  flex-wrap:wrap;
  gap:.4rem;
  justify-content:center;
  max-width:340px;
}
.s-opt{
  font-size:1.9rem;
  cursor:pointer;
  padding:.2rem;
  border-radius:8px;
  line-height:1;
  transition:all .15s ease;
}
.s-opt:hover{transform:scale(1.45);background:var(--cream2);}
.s-opt.active{background:rgba(200,152,48,.2);transform:scale(1.2);outline:2px solid var(--gold);border-radius:8px;}
.picker-close{
  font-family:'Caveat',cursive;
  font-size:.9rem;
  color:var(--text-light);
  background:none;border:none;
  cursor:pointer;text-decoration:underline;
  padding:.2rem .5rem;
}

/* ── UPLOAD MODAL ───────────────────────────────────────── */
.overlay{
  position:fixed;inset:0;
  background:rgba(58,40,16,.65);
  backdrop-filter:blur(5px);
  z-index:400;
  display:none;
  align-items:center;justify-content:center;
}
.overlay.open{display:flex;}
.modal{
  background:var(--cream);
  border-radius:18px;
  padding:2rem 2.2rem 1.8rem;
  max-width:460px;width:92vw;
  box-shadow:0 25px 70px rgba(0,0,0,.35);
  position:relative;
}
.modal-title{
  font-family:'Caveat',cursive;
  font-size:1.9rem;font-weight:700;
  color:var(--rose-dark);
  margin-bottom:1.2rem;
}
.modal-close{
  position:absolute;top:.9rem;right:1rem;
  background:none;border:none;
  font-size:1.3rem;cursor:pointer;
  color:var(--text-light);
  width:34px;height:34px;border-radius:50%;
  display:flex;align-items:center;justify-content:center;
  transition:background .2s;
}
.modal-close:hover{background:var(--cream2);}
.drop-area{
  position:relative;
  border:2px dashed var(--kraft-light);
  border-radius:12px;
  padding:2rem 1.5rem;
  text-align:center;
  cursor:pointer;
  transition:all .2s;
}
.drop-area:hover,.drop-area.over{
  border-color:var(--rose);
  background:rgba(212,120,110,.04);
}
.drop-area input{position:absolute;inset:0;opacity:0;cursor:pointer;width:100%;height:100%;}
.drop-icon{font-size:2.5rem;display:block;margin-bottom:.4rem;}
.drop-area p{font-family:'Caveat',cursive;font-size:1.05rem;color:var(--text-mid);}
.drop-area small{font-size:.8rem;color:var(--text-light);}
.modal-btn{
  display:block;width:100%;
  margin-top:1rem;
  background:var(--rose-dark);color:#fff;
  border:none;padding:.75rem;
  border-radius:50px;
  font-family:'Caveat',cursive;font-size:1.2rem;font-weight:700;
  cursor:pointer;transition:all .2s;
}
.modal-btn:hover{filter:brightness(1.1);}
.modal-btn:disabled{opacity:.5;cursor:not-allowed;}

/* ── LIGHTBOX ───────────────────────────────────────────── */
.lightbox{
  position:fixed;inset:0;
  background:rgba(20,10,5,.93);
  z-index:600;
  display:flex;align-items:center;justify-content:center;
  opacity:0;pointer-events:none;
  transition:opacity .3s;
}
.lightbox.open{opacity:1;pointer-events:all;}
.lightbox img{
  max-width:min(90vw,960px);
  max-height:87vh;
  object-fit:contain;
  border-radius:4px;
  box-shadow:0 25px 70px rgba(0,0,0,.6);
  transform:scale(.93);
  transition:transform .3s cubic-bezier(.34,1.56,.64,1);
}
.lightbox.open img{transform:scale(1);}
.lb-close{
  position:absolute;top:1.2rem;right:1.4rem;
  background:rgba(255,255,255,.15);border:none;
  color:#fff;font-size:1.5rem;
  width:42px;height:42px;border-radius:50%;
  cursor:pointer;
  display:flex;align-items:center;justify-content:center;
  transition:background .2s;
}
.lb-close:hover{background:rgba(255,255,255,.28);}

/* ── TOAST ──────────────────────────────────────────────── */
.toast{
  position:fixed;bottom:2rem;left:50%;
  transform:translateX(-50%) translateY(100px);
  background:var(--text);color:#fff;
  padding:.65rem 1.6rem;
  border-radius:50px;
  font-family:'Caveat',cursive;font-size:1.1rem;
  transition:transform .4s cubic-bezier(.34,1.56,.64,1);
  z-index:700;white-space:nowrap;
  box-shadow:0 4px 20px rgba(0,0,0,.3);
}
.toast.show{transform:translateX(-50%) translateY(0);}
.toast.ok{background:#4a7a5a;}
.toast.err{background:#8a3a3a;}

/* ── RESPONSIVE ─────────────────────────────────────────── */
@media(max-width:620px){
  .cover-card{padding:2rem 1.5rem;}
  .cover-buttons{top:.8rem;right:.8rem;gap:.4rem;}
  .cover-btn{padding:.45rem .8rem;font-size:.88rem;}
  .photos-row{grid-template-columns:1fr;gap:2rem;}
  .photo-slot:nth-child(even){margin-top:0;}
  .binding{width:36px;}
  .page{padding:1.3rem 1rem 1.3rem;}
}
</style>
</head>
<body>

<!-- ═══ COVER ═══════════════════════════════════════════════ -->
<div class="cover" id="cover">
  <!-- patchwork quilt background -->
  <div class="cover-bg" id="coverBg"></div>

  <!-- top-right buttons -->
  <div class="cover-buttons">
    <button class="cover-btn btn-add-photos" onclick="openUpload()">📷 Adicionar Fotos</button>
    <button class="cover-btn btn-add-stickers" id="stickerModeBtn" onclick="toggleStickerMode()">✨ Figurinhas</button>
  </div>

  <!-- flowers -->
  <span class="flower" style="font-size:3.5rem;top:7%;left:5%;animation-delay:0s">🌸</span>
  <span class="flower" style="font-size:2rem;top:14%;left:20%;animation-delay:.5s">🌼</span>
  <span class="flower" style="font-size:2.8rem;top:6%;right:18%;animation-delay:1s">🌺</span>
  <span class="flower" style="font-size:2rem;top:22%;right:5%;animation-delay:.3s">🌷</span>
  <span class="flower" style="font-size:3rem;bottom:18%;left:7%;animation-delay:.8s">🌻</span>
  <span class="flower" style="font-size:1.8rem;bottom:25%;left:25%;animation-delay:.2s">🌸</span>
  <span class="flower" style="font-size:3.2rem;bottom:12%;right:8%;animation-delay:1.2s">🌺</span>
  <span class="flower" style="font-size:1.6rem;bottom:28%;right:22%;animation-delay:.7s">🌼</span>
  <span class="flower" style="font-size:2.2rem;top:42%;left:2%;animation-delay:.9s">🌷</span>
  <span class="flower" style="font-size:2.5rem;top:55%;right:2%;animation-delay:.4s">🌸</span>
  <span class="flower" style="font-size:1.5rem;top:30%;left:10%;animation-delay:1.4s">🌼</span>
  <span class="flower" style="font-size:1.5rem;bottom:38%;right:14%;animation-delay:1.1s">🌷</span>

  <!-- title card -->
  <div class="cover-card">
    <!-- tape pieces on card -->
    <div class="card-tape tape-gold" style="width:70px;top:-11px;left:18%;transform:rotate(-4deg)"></div>
    <div class="card-tape tape-teal" style="width:60px;top:-11px;right:22%;transform:rotate(3deg)"></div>
    <div class="card-tape tape-rose" style="width:55px;bottom:-11px;left:50%;transform:translateX(-50%) rotate(-2deg)"></div>

    <span class="cover-deco">🌸 🌸 🌸</span>
    <div class="cover-title">Álbum da Dona Nenê</div>
    <div class="cover-sub">Um álbum de memórias eternas ✦ Feito com amor</div>
    <div class="cover-badge">🎂 85 Anos de Alegria</div>
  </div>
</div>

<!-- ═══ ALBUM ════════════════════════════════════════════════ -->
<div class="album">

  <!-- Section 1: Permanent photos of Nenê -->
  <div class="section-banner">
    <div class="section-banner-text">🌸 Memórias da Nenê 🌸</div>
  </div>
  <div id="permanent-container"></div>

  <!-- Section 2: Party photos -->
  <div class="section-banner">
    <div class="section-banner-text">🎂 Níver de 85 Anos 🎂</div>
  </div>
  <div id="party-container"></div>

</div>

<!-- ═══ STICKER PICKER ═══════════════════════════════════════ -->
<div class="sticker-picker" id="stickerPicker">
  <div class="picker-label">✨ Escolha uma figurinha</div>
  <div class="sticker-grid" id="stickerGrid"></div>
  <div class="picker-hint" id="pickerHint">Selecione uma figurinha acima, depois clique na página</div>
  <button class="picker-close" onclick="closeStickerMode()">✕ Encerrar modo figurinha</button>
</div>

<!-- ═══ UPLOAD MODAL ══════════════════════════════════════════ -->
<div class="overlay" id="uploadOverlay">
  <div class="modal">
    <button class="modal-close" onclick="closeUpload()">✕</button>
    <div class="modal-title">📷 Adicionar Fotos à Festa</div>
    <div class="drop-area" id="dropArea">
      <input type="file" id="fileInput" accept="image/*" multiple/>
      <span class="drop-icon">📂</span>
      <p>Clique ou arraste as fotos aqui</p>
      <small>JPG, PNG, GIF, WebP — máx. 50MB por arquivo</small>
    </div>
    <button class="modal-btn" id="uploadBtn" disabled onclick="doUpload()">✦ Enviar fotos para o álbum</button>
  </div>
</div>

<!-- ═══ LIGHTBOX ═════════════════════════════════════════════ -->
<div class="lightbox" id="lightbox" onclick="if(event.target===this)closeLightbox()">
  <button class="lb-close" onclick="closeLightbox()">✕</button>
  <img id="lbImg" src="" alt=""/>
</div>

<div class="toast" id="toast"></div>

<script>
// ── Constants ─────────────────────────────────────────────
var STICKERS_NENE  = ['❤️','💕','🌸','🌺','🌼','⭐','✨','🦋','🌈','💫','🎀','💖','🌷','🍃','☀️','🌙','🫶','💐','🩷','🌻'];
var STICKERS_PARTY = ['🎂','🎉','🎈','🎁','🎀','🥳','🌹','💐','✨','⭐','🎊','🍰','🥂','💕','🎶','🪷','🫶','🎠','🎵','🥰','💝','🌸'];

// ── State ─────────────────────────────────────────────────
var stickerMode   = false;
var pickedSticker = null;
var allStickers   = [];
var allComments   = [];
var selectedFiles = null;
var toastTimer    = null;

// ── Toast ─────────────────────────────────────────────────
function toast(msg, cls) {
  var el = document.getElementById('toast');
  clearTimeout(toastTimer);
  el.textContent = msg;
  el.className = 'toast show ' + (cls || '');
  toastTimer = setTimeout(function(){ el.className = 'toast'; }, 3000);
}

// ── Escape HTML ───────────────────────────────────────────
function esc(s) {
  return String(s)
    .replace(/&/g,'&amp;').replace(/</g,'&lt;')
    .replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

// ── Cover patchwork ───────────────────────────────────────
(function buildCoverBg(){
  var bg = document.getElementById('coverBg');
  for (var i = 0; i < 20; i++) {
    var d = document.createElement('div');
    d.className = 'cover-patch';
    bg.appendChild(d);
  }
})();

// ── Upload ────────────────────────────────────────────────
function openUpload() { document.getElementById('uploadOverlay').classList.add('open'); }
function closeUpload() {
  document.getElementById('uploadOverlay').classList.remove('open');
  document.getElementById('fileInput').value = '';
  document.getElementById('uploadBtn').disabled = true;
  selectedFiles = null;
}
document.getElementById('uploadOverlay').addEventListener('click', function(e) {
  if (e.target === this) closeUpload();
});

var fileInput = document.getElementById('fileInput');
fileInput.addEventListener('change', function() {
  selectedFiles = fileInput.files;
  document.getElementById('uploadBtn').disabled = !selectedFiles || !selectedFiles.length;
  if (selectedFiles && selectedFiles.length) toast(selectedFiles.length + ' foto(s) selecionada(s) ✓', 'ok');
});

var dropArea = document.getElementById('dropArea');
dropArea.addEventListener('dragover', function(e){ e.preventDefault(); dropArea.classList.add('over'); });
dropArea.addEventListener('dragleave', function(){ dropArea.classList.remove('over'); });
dropArea.addEventListener('drop', function(e) {
  e.preventDefault();
  dropArea.classList.remove('over');
  var files = Array.from(e.dataTransfer.files).filter(function(f){ return f.type.startsWith('image/'); });
  if (!files.length) { toast('Apenas imagens são aceitas', 'err'); return; }
  var dt = new DataTransfer();
  files.forEach(function(f){ dt.items.add(f); });
  fileInput.files = dt.files;
  selectedFiles = fileInput.files;
  document.getElementById('uploadBtn').disabled = false;
  toast(files.length + ' foto(s) pronta(s) ✓', 'ok');
});

async function doUpload() {
  if (!selectedFiles || !selectedFiles.length) return;
  var btn = document.getElementById('uploadBtn');
  btn.disabled = true;
  btn.textContent = 'Enviando...';
  var fd = new FormData();
  for (var i = 0; i < selectedFiles.length; i++) fd.append('images', selectedFiles[i]);
  try {
    var res = await fetch('/api/party/upload', { method: 'POST', body: fd });
    var data = await res.json();
    toast(data.uploaded + ' foto(s) adicionada(s) ao álbum! 🎉', 'ok');
    closeUpload();
    await loadParty();
  } catch(e) {
    toast('Erro ao enviar fotos', 'err');
  }
  btn.disabled = false;
  btn.textContent = '✦ Enviar fotos para o álbum';
}

// ── Lightbox ──────────────────────────────────────────────
function openLightbox(src) {
  document.getElementById('lbImg').src = src;
  document.getElementById('lightbox').classList.add('open');
  document.body.style.overflow = 'hidden';
}
function closeLightbox() {
  document.getElementById('lightbox').classList.remove('open');
  document.body.style.overflow = '';
  setTimeout(function(){ document.getElementById('lbImg').src = ''; }, 300);
}
document.addEventListener('keydown', function(e){ if(e.key==='Escape'){closeLightbox();closeStickerMode();} });

// ── Sticker mode ──────────────────────────────────────────
function toggleStickerMode() {
  if (stickerMode) { closeStickerMode(); return; }
  stickerMode = true;
  document.getElementById('stickerPicker').classList.add('open');
  document.getElementById('stickerModeBtn').textContent = '✕ Sair de Figurinhas';
  document.querySelectorAll('.spread').forEach(function(s){ s.classList.add('sticker-mode'); });
  renderStickerPicker();
  toast('Selecione uma figurinha e clique em qualquer página ✨');
}

function closeStickerMode() {
  stickerMode = false;
  pickedSticker = null;
  document.getElementById('stickerPicker').classList.remove('open');
  document.getElementById('stickerModeBtn').textContent = '✨ Figurinhas';
  document.querySelectorAll('.spread').forEach(function(s){ s.classList.remove('sticker-mode'); });
}

function renderStickerPicker() {
  var merged = STICKERS_NENE.concat(STICKERS_PARTY.filter(function(s){ return STICKERS_NENE.indexOf(s) < 0; }));
  var grid = document.getElementById('stickerGrid');
  grid.innerHTML = '';
  merged.forEach(function(emoji) {
    var span = document.createElement('span');
    span.className = 's-opt';
    span.textContent = emoji;
    span.addEventListener('click', function() {
      document.querySelectorAll('.s-opt').forEach(function(s){ s.classList.remove('active'); });
      span.classList.add('active');
      pickedSticker = emoji;
      document.getElementById('pickerHint').textContent = emoji + ' selecionada! Clique numa página para colocar.';
    });
    grid.appendChild(span);
  });
}

// ── Build page spread ─────────────────────────────────────
function buildSpread(photos, pageId, section, withComments) {
  var spread = document.createElement('div');
  spread.className = 'spread';
  spread.dataset.pageId = pageId;

  // binding
  var binding = document.createElement('div');
  binding.className = 'binding';
  for (var i = 0; i < 10; i++) {
    var strip = document.createElement('div');
    strip.className = 'b-strip';
    binding.appendChild(strip);
  }
  // rings at 20%, 50%, 80%
  [20, 50, 80].forEach(function(pct) {
    var ring = document.createElement('div');
    ring.className = 'b-ring';
    ring.style.top = pct + '%';
    binding.appendChild(ring);
  });
  spread.appendChild(binding);

  // page
  var page = document.createElement('div');
  page.className = 'page';

  // photos row
  var row = document.createElement('div');
  row.className = 'photos-row';

  var tapes = ['tape-a','tape-b','tape-c'];
  photos.forEach(function(photo, idx) {
    var slot = document.createElement('div');
    slot.className = 'photo-slot';

    var frame = document.createElement('div');
    frame.className = 'photo-frame';
    frame.title = 'Clique para ampliar';

    // tape
    var tape = document.createElement('div');
    tape.className = 'tape ' + tapes[(idx + Math.floor(Math.random()*3)) % 3];
    frame.appendChild(tape);

    // corners
    ['ctlY','ctr','cbl','cbr'].forEach(function(cls) {
      var c = document.createElement('div');
      c.className = 'corner ' + cls;
      frame.appendChild(c);
    });

    var img = document.createElement('img');
    img.src = photo.url;
    img.alt = 'Foto';
    if (!photo.url.toLowerCase().endsWith('.gif')) {
    img.loading = 'lazy';
    }
    frame.appendChild(img);

    // click to zoom
    frame.addEventListener('click', function(e) {
      if (stickerMode) return;
      openLightbox(photo.url);
    });

    // download
    var dl = document.createElement('a');
    var sec = section === 'permanent' ? 'permanent' : 'party';
    dl.href = '/download/' + sec + '/' + photo.name;
    dl.download = photo.name;
    dl.className = 'photo-dl';
    dl.textContent = '↓ baixar';
    dl.addEventListener('click', function(e){ e.stopPropagation(); });
    frame.appendChild(dl);

    slot.appendChild(frame);
    row.appendChild(slot);
  });

  page.appendChild(row);

  // comments (party only)
  if (withComments) {
    var ca = buildCommentsArea(pageId);
    page.appendChild(ca);
  }

  spread.appendChild(page);

  // stickers layer
  var sl = document.createElement('div');
  sl.className = 'stickers-layer';
  spread.appendChild(sl);

  // place existing stickers
  allStickers.filter(function(s){ return s.pageId === pageId; })
    .forEach(function(s){ placeSticker(spread, sl, s); });

  // click to add sticker
  spread.addEventListener('click', async function(e) {
    if (!stickerMode || !pickedSticker) return;
    if (e.target.closest('.comment-form,.btn-new-comment,.sticker-picker,.overlay,.photo-dl')) return;

    var rect = spread.getBoundingClientRect();
    var x = ((e.clientX - rect.left) / rect.width)  * 100;
    var y = ((e.clientY - rect.top)  / rect.height) * 100;

    var payload = { pageId: pageId, section: section, emoji: pickedSticker, x: x, y: y };
    try {
      var res = await fetch('/api/stickers', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      var saved = await res.json();
      allStickers.push(saved);
      placeSticker(spread, sl, saved);
      toast(pickedSticker + ' colocada!', 'ok');
    } catch(e) {
      toast('Erro ao salvar figurinha', 'err');
    }
  });

  return spread;
}

function placeSticker(spread, layer, s) {
  var rot = (Math.random() * 30 - 15).toFixed(1);
  var el = document.createElement('span');
  el.className = 'sticker-el';
  el.textContent = s.emoji;
  el.title = 'Duplo clique para remover';
  el.style.left  = s.x + '%';
  el.style.top   = s.y + '%';
  el.style.transform = 'translate(-50%,-50%) rotate(' + rot + 'deg)';
  el.addEventListener('dblclick', async function(e) {
    e.stopPropagation();
    if (!confirm('Remover esta figurinha?')) return;
    try {
      await fetch('/api/stickers/' + s.id, { method: 'DELETE' });
      el.remove();
      allStickers = allStickers.filter(function(x){ return x.id !== s.id; });
      toast('Figurinha removida');
    } catch { toast('Erro', 'err'); }
  });
  layer.appendChild(el);
}

// ── Comments area ─────────────────────────────────────────
function buildCommentsArea(pageId) {
  var ca = document.createElement('div');
  ca.className = 'comments-area';

  var title = document.createElement('div');
  title.className = 'comments-title';
  title.textContent = '💬 Mensagens sobre esta página';
  ca.appendChild(title);

  var list = document.createElement('div');
  list.className = 'comment-list';
  ca.appendChild(list);

  // render existing comments for this page
  allComments.filter(function(c){ return c.pageId === pageId; })
    .forEach(function(c){ appendComment(list, c); });

  var btnNew = document.createElement('button');
  btnNew.className = 'btn-new-comment';
  btnNew.innerHTML = '✏️ Deixar mensagem';
  ca.appendChild(btnNew);

  var form = document.createElement('div');
  form.className = 'comment-form';
  form.innerHTML =
    '<input class="cf-input" placeholder="Seu nome" maxlength="60"/>' +
    '<textarea class="cf-textarea" placeholder="Escreva uma mensagem sobre a festa..." maxlength="400"></textarea>' +
    '<button class="cf-submit">Enviar 💌</button>';
  ca.appendChild(form);

  btnNew.addEventListener('click', function() {
    form.classList.toggle('open');
    if (form.classList.contains('open')) form.querySelector('.cf-input').focus();
  });

  form.querySelector('.cf-submit').addEventListener('click', async function(e) {
    e.stopPropagation();
    var name = form.querySelector('.cf-input').value.trim();
    var text = form.querySelector('.cf-textarea').value.trim();
    if (!name || !text) { toast('Preencha nome e mensagem', 'err'); return; }
    try {
      var res = await fetch('/api/comments', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ pageId: pageId, name: name, text: text })
      });
      var saved = await res.json();
      allComments.push(saved);
      appendComment(list, saved);
      form.classList.remove('open');
      form.querySelector('.cf-input').value  = '';
      form.querySelector('.cf-textarea').value = '';
      toast('Mensagem enviada! 💌', 'ok');
    } catch { toast('Erro ao enviar mensagem', 'err'); }
  });

  return ca;
}

function appendComment(list, c) {
  var el = document.createElement('div');
  el.className = 'comment-card';
  el.innerHTML =
    '<div class="comment-header">' +
      '<span class="comment-name">' + esc(c.name) + '</span>' +
      '<span class="comment-date">' + esc(c.date || '') + '</span>' +
    '</div>' +
    '<div class="comment-body">' + esc(c.text) + '</div>';
  list.appendChild(el);
}

// ── Render album (paginated) ──────────────────────────────
function renderAlbum(photos, containerId, section, withComments) {
  var container = document.getElementById(containerId);
  container.innerHTML = '';

  if (!photos || !photos.length) {
    var wrapper = document.createElement('div');
    wrapper.className = 'pag-wrapper';

    var es = document.createElement('div');
    es.className = 'empty-spread';
    var eb = document.createElement('div');
    eb.className = 'binding';
    for (var i = 0; i < 8; i++) {
      var s = document.createElement('div');
      s.className = 'b-strip';
      eb.appendChild(s);
    }
    es.appendChild(eb);
    var ep = document.createElement('div');
    ep.className = 'empty-page';
    ep.innerHTML = '<span>📷</span><p>' + (section === 'permanent'
      ? 'Adicione fotos da Nenê na pasta uploads/permanent'
      : 'Clique em "Adicionar Fotos" para começar o álbum da festa!') + '</p>';
    es.appendChild(ep);
    wrapper.appendChild(es);
    container.appendChild(wrapper);
    return;
  }

  // Build all spreads inside a wrapper
  var wrapper = document.createElement('div');
  wrapper.className = 'pag-wrapper';

  var spreads = [];
  for (var i = 0; i < photos.length; i += 2) {
    var pair   = photos.slice(i, i + 2);
    var pageId = section + '-p' + Math.floor(i / 2);
    var spread = buildSpread(pair, pageId, section, withComments);
    wrapper.appendChild(spread);
    spreads.push(spread);
  }

  // Navigation bar
  var nav = document.createElement('div');
  nav.className = 'pag-nav';

  var btnPrev = document.createElement('button');
  btnPrev.className = 'pag-arrow';
  btnPrev.innerHTML = '←';
  btnPrev.title = 'Página anterior';

  var label = document.createElement('span');
  label.className = 'pag-label';

  var btnNext = document.createElement('button');
  btnNext.className = 'pag-arrow';
  btnNext.innerHTML = '→';
  btnNext.title = 'Próxima página';

  nav.appendChild(btnPrev);
  nav.appendChild(label);
  nav.appendChild(btnNext);

  var current = 0;

  function showPage(idx) {
    spreads.forEach(function(sp){ sp.classList.remove('active'); });
    spreads[idx].classList.add('active');
    label.textContent = 'Página ' + (idx + 1) + ' / ' + spreads.length;
    btnPrev.disabled = idx === 0;
    btnNext.disabled = idx === spreads.length - 1;
    current = idx;
    // smooth scroll to wrapper top
    wrapper.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
  }

  btnPrev.addEventListener('click', function(){ if (current > 0) showPage(current - 1); });
  btnNext.addEventListener('click', function(){ if (current < spreads.length - 1) showPage(current + 1); });

  showPage(0);

  wrapper.appendChild(nav);
  container.appendChild(wrapper);
}

// ── Load data ─────────────────────────────────────────────
async function loadParty() {
  var res    = await fetch('/api/party');
  var photos = await res.json();
  renderAlbum(photos, 'party-container', 'party', true);
}

async function loadAll() {
  try {
    var [permRes, partyRes, stickerRes, commentRes] = await Promise.all([
      fetch('/api/permanent'),
      fetch('/api/party'),
      fetch('/api/stickers'),
      fetch('/api/comments'),
    ]);
    var permPhotos  = await permRes.json();
    var partyPhotos = await partyRes.json();
    allStickers     = await stickerRes.json();
    allComments     = await commentRes.json();

    renderAlbum(permPhotos,  'permanent-container', 'permanent', false);
    renderAlbum(partyPhotos, 'party-container',     'party',     true);
  } catch(e) {
    toast('Erro ao carregar o álbum', 'err');
    console.error(e);
  }
}

loadAll();
</script>
</body>
</html>`