package i18n

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDefaultCatalog_english(t *testing.T) {
	c := DefaultCatalog()
	if got := c.T("auth.welcome"); got != "Welcome!" {
		t.Errorf("T(auth.welcome) = %q", got)
	}
	if got := c.T("auth.signup_prompt"); got != "Don't have an account?" {
		t.Errorf("T(auth.signup_prompt) = %q", got)
	}
}

func TestNewCatalog_portuguese(t *testing.T) {
	c := NewCatalog("pt-BR")
	if got := c.T("auth.welcome"); got != "Bem-vindo!" {
		t.Errorf("T(auth.welcome) = %q", got)
	}
	if c.HTMLLang() != "pt-BR" {
		t.Errorf("HTMLLang() = %q, want pt-BR", c.HTMLLang())
	}
}

// Both catalogs must carry the same keys: a missing key renders its own name
// on the page instead of failing the build.
func TestCatalogsHaveSameKeys(t *testing.T) {
	for key := range enMessages {
		if _, ok := ptMessages[key]; !ok {
			t.Errorf("pt is missing %q", key)
		}
	}
	for key := range ptMessages {
		if _, ok := enMessages[key]; !ok {
			t.Errorf("en is missing %q", key)
		}
	}
}

// Guard the templates: every static t "key" must exist in both catalogs.
func TestTemplateKeysExist(t *testing.T) {
	root := filepath.Join("..", "..", "web", "templates")
	pattern := regexp.MustCompile(`t\s+"([a-zA-Z0-9_.]+)"`)
	checked := 0

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range pattern.FindAllStringSubmatch(string(content), -1) {
			key := match[1]
			checked++
			if _, ok := enMessages[key]; !ok {
				t.Errorf("%s: en is missing %q", path, key)
			}
			if _, ok := ptMessages[key]; !ok {
				t.Errorf("%s: pt is missing %q", path, key)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("no t \"key\" calls found — the scan is broken")
	}
}
