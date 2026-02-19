# Gmail Login Scraper (Go + chromedp)

Scraper headless para login no Gmail, pronto para rodar em VPS.

## Dependências

### Instalar Go

```bash
sudo apt install golang-go
```

### Instalar Chrome no servidor (VPS/Ubuntu)

```bash
wget https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb
sudo apt install ./google-chrome-stable_current_amd64.deb
```

Ou via apt:

```bash
curl -fsSL https://dl.google.com/linux/linux_signing_key.pub | sudo gpg --dearmor -o /usr/share/keyrings/google-chrome.gpg
echo "deb [arch=amd64 signed-by=/usr/share/keyrings/google-chrome.gpg] http://dl.google.com/linux/chrome/deb/ stable main" | sudo tee /etc/apt/sources.list.d/google-chrome.list
sudo apt update && sudo apt install google-chrome-stable
```

## Uso

```bash
# 1. Baixar dependências
go mod tidy

# 2. Rodar o projeto
go run main.go
```

## Etapas do login (step-by-step)

| Etapa | O que faz                                                     |
| ----- | ------------------------------------------------------------- |
| 1     | Navega para `admanager.google.com` e aguarda o campo de email |
| 2     | Digita o email e clica em **Próxima**                         |
| 3     | Aguarda o campo de senha, digita e clica em **Próxima**       |
| 4     | Aguarda o carregamento da inbox do Gmail                      |
| ✅    | Captura os cookies de sessão                                  |

## Flags importantes para VPS

```go
chromedp.Flag("no-sandbox", true)            // obrigatório em Docker/VPS
chromedp.Flag("disable-dev-shm-usage", true) // evita crash por falta de memória
chromedp.Flag("disable-gpu", true)           // sem GPU no servidor
chromedp.Flag("headless", true)              // sem interface gráfica
```

## ⚠️ Aviso sobre 2FA / Verificação do Google

Se a conta tiver **verificação em 2 etapas** ativada, o scraper vai falhar no Step 4.
Soluções:

- Desativar 2FA para automação (não recomendado para contas pessoais)
- Usar uma conta de serviço do Google
- Tratar o fluxo de 2FA com `chromedp.WaitVisible` adicional e injeção do código

## Usar os cookies capturados

```go
// Exemplo: reutilizar cookies em outra requisição HTTP
jar, _ := cookiejar.New(nil)
for _, c := range cookies {
    jar.SetCookies(gmailURL, []*http.Cookie{{
        Name:  c.Name,
        Value: c.Value,
    }})
}
client := &http.Client{Jar: jar}
resp, _ := client.Get("https://mail.google.com/mail/u/0/")
```
