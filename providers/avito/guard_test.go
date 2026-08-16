package avito

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Полная страница Авито сохраняется из браузера ПОД ЛОГИНОМ и несёт в себе не
// только объявление: там лежат email и имя того, кто её сохранил, плюс имя
// продавца в запакованном виде. Репозиторий публичный, а из истории git такое
// убирается только её переписыванием.
//
// .gitignore прикрывает две конкретные папки (/pages/, /avito-pages/), но
// человек положит страницы туда, куда ему удобно. Поэтому проверяем не имя
// папки, а содержимое: маркер состояния Авито в любом отслеживаемом .html вне
// каталога фикстур — это случайно добавленная полная страница.
func TestNoFullPagesTracked(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Skip("не репозиторий git — страж пропущен:", err)
	}

	out, err := exec.Command("git", "-C", root, "ls-files", "-z", "--", "*.html", "*.htm").Output()
	if err != nil {
		t.Skip("git ls-files недоступен:", err)
	}

	const fixtures = "providers/avito/testdata/"
	for _, rel := range strings.Split(string(out), "\x00") {
		if rel == "" || strings.HasPrefix(rel, fixtures) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			continue // файл удалён из дерева, но ещё в индексе
		}
		if strings.Contains(string(data), hydrationVar) {
			t.Errorf("под git добавлена полная страница Авито: %s\n"+
				"В ней лежат email и имя человека, сохранившего страницу. "+
				"Уберите файл из индекса (git rm --cached) и добавьте его папку в .gitignore; "+
				"для тестов используйте вырезки из providers/avito/testdata.", rel)
		}
	}
}

func repoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
