package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/joho/godotenv"
)

type Config struct {
	Email    string
	Password string
	Headless bool
	Timeout  time.Duration
}

type Cookie struct {
	Name     string
	Value    string
	Domain   string
	Path     string
	Secure   bool
	HTTPOnly bool
}

func chromePath() string {
	switch runtime.GOOS {
	case "windows":
		paths := []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			os.ExpandEnv(`${LOCALAPPDATA}\Google\Chrome\Application\chrome.exe`),
		}
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	case "darwin":
		return "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	}
	return "google-chrome"
}

const screenshotsDir = "/app/screenshots"

func screenshot(ctx context.Context, filename string) {
	var buf []byte
	if err := chromedp.Run(ctx, chromedp.FullScreenshot(&buf, 90)); err != nil {
		log.Printf("⚠️  Screenshot falhou (%s): %v", filename, err)
		return
	}
	fullPath := screenshotsDir + "/" + filename
	if err := os.WriteFile(fullPath, buf, 0644); err != nil {
		log.Printf("⚠️  Erro ao salvar screenshot (%s): %v", fullPath, err)
		return
	}
	log.Printf("📸 Screenshot salvo: %s", fullPath)
}

func askInput(prompt string) string {
	fmt.Print(prompt)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	return strings.TrimSpace(scanner.Text())
}

// detectScreen fica em loop até encontrar a inbox ou o 2FA, retorna "inbox" ou "2fa"
func detectScreen(ctx context.Context) string {
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		var found int

		// checa inbox
		chromedp.Run(ctx, chromedp.Evaluate(`document.querySelectorAll('div[role="main"]').length`, &found))
		if found > 0 {
			return "inbox"
		}

		// checa 2FA
		chromedp.Run(ctx, chromedp.Evaluate(`document.querySelectorAll('[data-challengetype]').length`, &found))
		if found > 0 {
			return "2fa"
		}

		time.Sleep(500 * time.Millisecond)
	}
	return "timeout"
}

func handle2FA(ctx context.Context) error {
	screenshot(ctx, "step_2fa_detected.png")

	if err := chromedp.Run(ctx, chromedp.Click(`#view-more`, chromedp.ByQuery), chromedp.Sleep(1*time.Second)); err != nil {
		chromedp.Run(ctx, chromedp.Click(`[jsname="Njthtb"]`, chromedp.ByQuery), chromedp.Sleep(1*time.Second))
	}
	screenshot(ctx, "step_2fa_options.png")

	for _, sel := range []string{`[data-challengetype="11"]`, `[data-challengetype="9"]`, `[data-challengetype="6"]`} {
		if err := chromedp.Run(ctx, chromedp.Click(sel, chromedp.ByQuery), chromedp.Sleep(1500*time.Millisecond)); err == nil {
			log.Println("📱 Opção SMS/código selecionada")
			break
		}
	}
	screenshot(ctx, "step_2fa_sms_selected.png")

	chromedp.Run(ctx,
		chromedp.WaitVisible(`input[type="tel"], input[type="text"], #totpPin`, chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
	)
	screenshot(ctx, "step_2fa_code_input.png")

	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("🔐 VERIFICAÇÃO EM 2 ETAPAS DETECTADA")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	code := askInput("👉 Digite o código recebido e pressione ENTER: ")
	if code == "" {
		return fmt.Errorf("código 2FA não informado")
	}

	var inputSel string
	for _, sel := range []string{`input[type="tel"]`, `#totpPin`, `input[type="text"]`} {
		if err := chromedp.Run(ctx, chromedp.WaitVisible(sel, chromedp.ByQuery)); err == nil {
			inputSel = sel
			break
		}
	}
	if inputSel == "" {
		screenshot(ctx, "step_2fa_input_not_found.png")
		return fmt.Errorf("campo para digitar código 2FA não encontrado")
	}

	if err := chromedp.Run(ctx,
		chromedp.Click(inputSel, chromedp.ByQuery),
		chromedp.SendKeys(inputSel, code, chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
	); err != nil {
		return fmt.Errorf("digitar código 2FA: %w", err)
	}

	for _, sel := range []string{`#totpNext`, `button[type="submit"]`, `[jsname="LgbsSe"]`} {
		if err := chromedp.Run(ctx, chromedp.Click(sel, chromedp.ByQuery), chromedp.Sleep(2*time.Second)); err == nil {
			break
		}
	}
	log.Println("✅ Código 2FA enviado")
	screenshot(ctx, "step_2fa_submitted.png")
	return nil
}

func saveCookies(cookies []Cookie) error {
	if err := os.MkdirAll(screenshotsDir, 0755); err != nil {
		return fmt.Errorf("criar diretório de saída: %w", err)
	}
	data, err := json.MarshalIndent(cookies, "", "  ")
	if err != nil {
		return fmt.Errorf("serializar cookies: %w", err)
	}
	path := screenshotsDir + "/cookies.json"
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("salvar arquivo: %w", err)
	}
	log.Printf("💾 Cookies salvos em: %s", path)
	return nil
}

func main() {
	if err := os.MkdirAll(screenshotsDir, 0755); err != nil {
		log.Printf("⚠️  Não foi possível criar diretório de screenshots: %v", err)
	}

	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  .env não encontrado, usando variáveis do sistema")
	}

	cfg := Config{
		Email:    os.Getenv("GMAIL_EMAIL"),
		Password: os.Getenv("GMAIL_PASSWORD"),
		Headless: true,
		Timeout:  5 * time.Minute,
	}

	if cfg.Email == "" || cfg.Password == "" {
		log.Fatal("❌ Defina GMAIL_EMAIL e GMAIL_PASSWORD no .env")
	}

	cookies, err := GmailLogin(cfg)
	if err != nil {
		log.Fatalf("Login falhou: %v", err)
	}

	fmt.Println("✅ Login realizado com sucesso!")
	fmt.Printf("📦 %d cookies capturados\n", len(cookies))
	for _, c := range cookies {
		v := c.Value
		if len(v) > 20 {
			v = v[:20] + "..."
		}
		fmt.Printf("  🍪 %s = %s (domain: %s)\n", c.Name, v, c.Domain)
	}
}

func GmailLogin(cfg Config) ([]Cookie, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath()),
		chromedp.Flag("headless", cfg.Headless),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("window-size", "1280,800"),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	ctx, cancelCtx := chromedp.NewContext(allocCtx, chromedp.WithLogf(func(format string, args ...interface{}) {}))
	defer cancelCtx()

	ctx, cancelTimeout := context.WithTimeout(ctx, cfg.Timeout)
	defer cancelTimeout()

	log.Println("🌐 Abrindo página...")

	if err := chromedp.Run(ctx,
		chromedp.Navigate("https://admanager.google.com/23128820367"),
		chromedp.WaitVisible(`input[type="email"]`, chromedp.ByQuery),
		chromedp.Sleep(1*time.Second),
	); err != nil {
		return nil, fmt.Errorf("etapa 1 (navegação): %w", err)
	}
	log.Println("📧 Etapa 1/4: Página de email carregada")
	screenshot(ctx, "step1_email_page.png")

	if err := chromedp.Run(ctx,
		chromedp.Click(`input[type="email"]`, chromedp.ByQuery),
		chromedp.Sleep(300*time.Millisecond),
		chromedp.SendKeys(`input[type="email"]`, cfg.Email, chromedp.ByQuery),
		chromedp.Sleep(800*time.Millisecond),
		chromedp.Click(`#identifierNext`, chromedp.ByQuery),
		chromedp.Sleep(2*time.Second),
	); err != nil {
		return nil, fmt.Errorf("etapa 2 (email): %w", err)
	}
	log.Println("⏭️  Etapa 2/4: Email enviado")
	screenshot(ctx, "step2_after_email.png")

	if err := chromedp.Run(ctx,
		chromedp.WaitVisible(`input[type="password"]`, chromedp.ByQuery),
		chromedp.Sleep(1500*time.Millisecond),
	); err != nil {
		screenshot(ctx, "step3_error_no_password_field.png")
		return nil, fmt.Errorf("etapa 3 (aguardar campo senha): %w", err)
	}

	if err := chromedp.Run(ctx,
		chromedp.Click(`input[type="password"]`, chromedp.ByQuery),
		chromedp.Sleep(300*time.Millisecond),
		chromedp.SendKeys(`input[type="password"]`, cfg.Password, chromedp.ByQuery),
		chromedp.Sleep(800*time.Millisecond),
		chromedp.Click(`#passwordNext`, chromedp.ByQuery),
		chromedp.Sleep(3*time.Second),
	); err != nil {
		return nil, fmt.Errorf("etapa 3 (senha): %w", err)
	}
	log.Println("🔑 Etapa 3/4: Senha enviada")
	screenshot(ctx, "step3_after_password.png")

	log.Println("🔍 Detectando próxima tela...")
	screen := detectScreen(ctx)
	log.Printf("📺 Tela detectada: %s", screen)

	switch screen {
	case "2fa":
		log.Println("🔐 2FA detectado, iniciando verificação...")
		if err := handle2FA(ctx); err != nil {
			return nil, err
		}
	case "inbox":
		log.Println("✅ Sem 2FA, inbox carregada diretamente!")
	default:
		screenshot(ctx, "step_timeout.png")
		return nil, fmt.Errorf("timeout: nenhuma tela reconhecida após a senha")
	}

	if err := chromedp.Run(ctx,
		chromedp.WaitVisible(`div[role="main"]`, chromedp.ByQuery),
		chromedp.Sleep(1*time.Second),
	); err != nil {
		screenshot(ctx, "step4_error.png")
		return nil, fmt.Errorf("etapa 4 (inbox): %w", err)
	}
	log.Println("📬 Etapa 4/4: Login concluído!")
	screenshot(ctx, "step4_success.png")

	var sessionCookies []Cookie
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		cookies, err := network.GetCookies().Do(ctx)
		if err != nil {
			return err
		}
		for _, c := range cookies {
			sessionCookies = append(sessionCookies, Cookie{
				Name:     c.Name,
				Value:    c.Value,
				Domain:   c.Domain,
				Path:     c.Path,
				Secure:   c.Secure,
				HTTPOnly: c.HTTPOnly,
			})
		}
		return nil
	})); err != nil {
		return nil, fmt.Errorf("captura de cookies: %w", err)
	}

	if err := saveCookies(sessionCookies); err != nil {
		log.Printf("⚠️  Erro ao salvar cookies: %v", err)
	}

	time.Sleep(5 * time.Minute)

	return sessionCookies, nil
}
