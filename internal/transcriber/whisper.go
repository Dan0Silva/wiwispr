package transcriber

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var (
	// Matches bracketed or parenthesized whisper artifacts like [Música], [Silêncio], (applause), etc.
	artifactBracketRegex = regexp.MustCompile(`(?i)(\[[^\]]*\]|\([^\)]*\)|\*[^\*]*\*)`)
	multiSpaceRegex      = regexp.MustCompile(`\s+`)

	// Known Portuguese/multilingual Whisper hallucinations during silence or breath
	hallucinationPhrases = []string{
		"legendas pela comunidade amara.org",
		"legendas por",
		"subtitles by",
		"obrigado por assistir",
		"inscreva-se no canal",
		"até o próximo vídeo",
		"música",
		"silêncio",
	}
)

// Transcribe executes whisper-cli on wavPath and returns cleaned Portuguese text.
func Transcribe(ctx context.Context, whisperBin, modelPath, wavPath, language string, threads int) (string, error) {
	if _, err := os.Stat(whisperBin); err != nil {
		return "", fmt.Errorf("whisper binary not found at %s (run './scripts/setup.sh' first): %w", whisperBin, err)
	}
	if _, err := os.Stat(modelPath); err != nil {
		return "", fmt.Errorf("whisper model not found at %s (run './scripts/setup.sh' first): %w", modelPath, err)
	}
	if _, err := os.Stat(wavPath); err != nil {
		return "", fmt.Errorf("audio file not found at %s: %w", wavPath, err)
	}

	args := []string{
		"-m", modelPath,
		"-l", language,
		"-f", wavPath,
		"-nt", // no timestamps
		"-np", // no prints other than results
		"-t", strconv.Itoa(threads),
	}

	cmd := exec.CommandContext(ctx, whisperBin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("whisper-cli failed: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	cleaned := CleanWhisperOutput(stdout.String())
	if cleaned == "" {
		return "", errors.New("empty transcription")
	}
	return cleaned, nil
}

// CleanWhisperOutput strips timestamps, audio event tags, and common silence hallucinations.
func CleanWhisperOutput(raw string) string {
	lines := strings.Split(raw, "\n")
	var validParts []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Remove bracketed/parenthesized annotations like [Música], (risos), [BLANK_AUDIO]
		line = artifactBracketRegex.ReplaceAllString(line, "")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		lower := strings.ToLower(line)
		isHallucination := false
		for _, h := range hallucinationPhrases {
			if lower == h || lower == h+"." || lower == h+"!" {
				isHallucination = true
				break
			}
		}
		if isHallucination {
			continue
		}

		validParts = append(validParts, line)
	}

	joined := strings.Join(validParts, " ")
	joined = multiSpaceRegex.ReplaceAllString(joined, " ")
	return strings.TrimSpace(joined)
}
