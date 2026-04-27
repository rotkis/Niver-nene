# 🌸 Galeria da Vovó

Site de galeria de fotos responsivo para o aniversário da vovó.  
Feito com **Go** puro — sem dependências externas.

---

## 🚀 Rodar localmente

**Pré-requisito:** [Go 1.22+](https://go.dev/dl/)

```bash
# Clone/entre na pasta do projeto
cd vovo-gallery

# Rodar o servidor
go run main.go

# Acesse: http://localhost:8080
```

---

## 🐳 Rodar com Docker

```bash
# Build da imagem
docker build -t vovo-gallery .

# Rodar (as fotos ficam salvas em ./uploads no seu computador)
docker run -p 8080:8080 -v $(pwd)/uploads:/app/uploads vovo-gallery
```

---

## ☁️ Deploy gratuito — Railway (recomendado, mais fácil)

1. Crie uma conta em [railway.app](https://railway.app)
2. Clique em **New Project → Deploy from GitHub**
3. Faça upload/push do projeto para um repositório GitHub
4. O Railway detecta o `Dockerfile` automaticamente e faz o deploy
5. Vá em **Settings → Networking → Generate Domain** para pegar seu link público

> ⚠️ **Importante:** O Railway tem storage efêmero. Para as fotos não sumirem ao reiniciar,
> adicione um **Volume** no Railway: vá em seu serviço → *Volumes* → *Add Volume* → Mount path: `/app/uploads`

---

## ☁️ Deploy — Fly.io (alternativa)

```bash
# Instale o CLI do Fly.io
curl -L https://fly.io/install.sh | sh

# Faça login
fly auth login

# Na pasta do projeto:
fly launch        # configura o app
fly volumes create uploads_data --size 1   # cria volume persistente
fly deploy
```

No arquivo `fly.toml` gerado, adicione o volume:
```toml
[mounts]
  source = "uploads_data"
  destination = "/app/uploads"
```

---

## ☁️ Deploy — Render (alternativa gratuita)

1. Crie conta em [render.com](https://render.com)
2. New → **Web Service** → conecte seu repositório GitHub
3. Environment: **Docker**
4. Render gera um link `.onrender.com` automaticamente

> Para persistência no Render, use um **Disk** (Storage) com mount `/app/uploads`

---

## 🌟 Funcionalidades

- ✅ Upload de múltiplas fotos (drag & drop ou clique)
- ✅ Galeria responsiva em grid/masonry
- ✅ Lightbox para ver foto em tela cheia
- ✅ Download de qualquer foto
- ✅ Excluir fotos
- ✅ Animações suaves
- ✅ Funciona no celular e computador
- ✅ Sem banco de dados — fotos ficam numa pasta `uploads/`
- ✅ Sem dependências externas — só Go puro

---

## 📁 Estrutura

```
vovo-gallery/
├── main.go        # Servidor + HTML embutido (tudo em um arquivo!)
├── go.mod         # Módulo Go
├── Dockerfile     # Para deploy em container
├── README.md      # Este arquivo
└── uploads/       # Criada automaticamente ao rodar
```

---

## 🔒 Senha de proteção (opcional)

Se quiser proteger o site com senha para que só a família acesse,
defina a variável de ambiente `GALLERY_PASSWORD`:

```bash
GALLERY_PASSWORD=aniversario2025 go run main.go
```

*(Implemente no `main.go` com um middleware simples de Basic Auth se necessário)*