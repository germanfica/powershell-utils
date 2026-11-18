
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func main() {
	var input string

	if len(os.Args) > 1 {
		input = strings.Join(os.Args[1:], " ")
	} else {
		fmt.Print("Ruta del video MP4: ")

		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			fmt.Fprintln(os.Stderr, "Error: no se recibió una ruta.")
			os.Exit(1)
		}
		input = scanner.Text()
	}

	input = strings.TrimSpace(input)
	input = strings.Trim(input, "\"'")

	if input == "" {
		fmt.Fprintln(os.Stderr, "Error: ruta vacía.")
		os.Exit(1)
	}

	if !strings.EqualFold(filepath.Ext(input), ".mp4") {
		fmt.Fprintln(os.Stderr, "Error: se requiere un archivo MP4.")
		os.Exit(1)
	}

	input, err := filepath.Abs(input)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	dir := filepath.Dir(input)
	name := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	wav := filepath.Join(dir, name+".wav")

	fmt.Println("\n1. Extraer audio WAV para Whisper\n")

	fmt.Printf(
		"ffmpeg -i %s \\\n"+
			"  -vn -ac 1 -ar 16000 -c:a pcm_s16le \\\n"+
			"  %s\n",
		shellQuote(input),
		shellQuote(wav),
	)

	fmt.Println("\n2. Transcribir japonés a texto con Whisper\n")

	fmt.Printf(
		"uv run whisper %s \\\n"+
			"  --model large-v3 \\\n"+
			"  --language Japanese \\\n"+
			"  --task transcribe \\\n"+
			"  --output_format txt \\\n"+
			"  --output_dir %s\n",
		shellQuote(wav),
		shellQuote(dir),
	)

	fmt.Printf("\nResultado: %s\n", filepath.Join(dir, name+".txt"))
}
