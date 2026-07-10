package agentruntime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

const (
	maxCatalogInstructionWords      = 1200
	maxCatalogControlledPromptWords = 1400
)

func TestCatalogPromptWordBudgets(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "global", "agents"))
	if err != nil {
		t.Fatalf("resolve global agents root: %v", err)
	}
	sharedWords := len(strings.Fields(
		renderBaseSystemPrompt(&model.Agent{Name: "Agent"}, model.Org{}, false, "") +
			"\n" + resolveCommunicationProfile("mimo-v2.5-pro").Content,
	))
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "instructions.md" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		instructionWords := len(strings.Fields(string(content)))
		if instructionWords > maxCatalogInstructionWords {
			t.Errorf("%s has %d instruction words, exceeds %d", path, instructionWords, maxCatalogInstructionWords)
		}
		if controlledWords := sharedWords + instructionWords; controlledWords > maxCatalogControlledPromptWords {
			t.Errorf("%s has %d controlled prompt words, exceeds %d", path, controlledWords, maxCatalogControlledPromptWords)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk global agent instructions: %v", err)
	}
}
