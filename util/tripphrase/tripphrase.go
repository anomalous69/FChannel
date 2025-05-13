// Inspired by http://worrydream.com/tripphrase
package tripphrase

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	wordListDir = "util/tripphrase"
	wordListExt = ".txt"
	minWordLen  = 2
)

var templates = []string{
	"verb article adj noun",
	"article adj adj noun",
	"article adv adj noun",
	"adv verb article noun",
}

var wordsByType = make(map[string][]string)

func templateForIndex(index int) string {
	return templates[index%len(templates)]
}

func wordForIndexAndType(index int, wordType string) (string, error) {
	words, err := wordsForType(wordType)
	if err != nil {
		return "", err
	}
	if len(words) == 0 {
		return "", fmt.Errorf("no words found for type: %s", wordType)
	}
	return words[index%len(words)], nil
}

func wordsForType(wordType string) ([]string, error) {
	if words, ok := wordsByType[wordType]; ok {
		return words, nil
	}

	filename := filepath.Join(wordListDir, wordType+wordListExt)
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open word list: %w", err)
	}
	defer file.Close()

	var words []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		word := scanner.Text()
		if len(word) >= minWordLen && !strings.Contains(word, "_") {
			words = append(words, word)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read word list: %w", err)
	}

	wordsByType[wordType] = words
	return words, nil
}

func GeneratePhrase(password string) (string, error) {
	hash := md5.Sum([]byte(password))
	hexHash := hex.EncodeToString(hash[:])

	var indexes []int
	for i := 0; i < len(hexHash); i += 4 {
		val := hexHash[i : i+4]
		num := 0
		fmt.Sscanf(val, "%x", &num)
		indexes = append(indexes, num)
	}

	template := templateForIndex(indexes[0])
	types := strings.Fields(template)

	var phraseWords []string
	for i, wordType := range types {
		if i+1 >= len(indexes) {
			return "", fmt.Errorf("not enough hash values for template")
		}
		word, err := wordForIndexAndType(indexes[i+1], wordType)
		if err != nil {
			return "", fmt.Errorf("failed to get word: %w", err)
		}
		phraseWords = append(phraseWords, word)
	}

	return "(" + strings.Join(phraseWords, " ") + ")", nil
}
