package main

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// getCipherKey pad ou tronque la cle a 32 octets (AES-256).
func getCipherKey(keyStr string) []byte {
	k := make([]byte, 32)
	copy(k, []byte(keyStr))
	return k
}

// EncryptValue chiffre une chaine de caracteres en AES-256-GCM avec un nonce de 12 octets.
func EncryptValue(keyStr, plaintext string) (string, error) {
	key := getCipherKey(keyStr)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("initialisation aes: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("initialisation gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generation nonce: %w", err)
	}

	sealed := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	payload := append(nonce, sealed...)
	return "enc:v1:" + base64.StdEncoding.EncodeToString(payload), nil
}

// DecryptValue dechiffre une chaine au format enc:v1:<base64>.
func DecryptValue(keyStr, encPayload string) (string, error) {
	if !strings.HasPrefix(encPayload, "enc:v1:") {
		return encPayload, nil
	}

	raw, err := base64.StdEncoding.DecodeString(encPayload[7:])
	if err != nil {
		return "", fmt.Errorf("decodage base64: %w", err)
	}

	if len(raw) < 28 {
		return "", errors.New("payload trop court (nonce 12 octets + tag 16 octets requis)")
	}

	key := getCipherKey(keyStr)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("initialisation aes: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("initialisation gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	nonce := raw[:nonceSize]
	ciphertext := raw[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("echec dechiffrement gcm (cle invalide ou payload corrompu): %w", err)
	}

	return string(plaintext), nil
}

// updateSecretsFile met a jour ou ajoute une paire cle=valeur dans un fichier YAML simple.
func updateSecretsFile(filePath, keyName, encryptedVal string) error {
	var lines []string
	foundKey := false
	inSecretsSection := false

	if data, err := os.ReadFile(filePath); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)

			if strings.HasPrefix(trimmed, "secrets:") || strings.HasPrefix(trimmed, "encrypted_secrets:") {
				inSecretsSection = true
				lines = append(lines, line)
				continue
			}

			if inSecretsSection && strings.HasPrefix(trimmed, keyName+":") {
				lines = append(lines, fmt.Sprintf("  %s: %s", keyName, encryptedVal))
				foundKey = true
				continue
			}

			lines = append(lines, line)
		}
	} else if os.IsNotExist(err) {
		lines = append(lines, "version: 1", "secrets:")
		inSecretsSection = true
	} else {
		return err
	}

	if !foundKey {
		if !inSecretsSection {
			lines = append(lines, "secrets:")
		}
		lines = append(lines, fmt.Sprintf("  %s: %s", keyName, encryptedVal))
	}

	output := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(filePath, []byte(output), 0644)
}

// viewSecretsFile lit et dechiffre les cles du fichier secrets.enc.yaml.
func viewSecretsFile(filePath, keyStr string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("lecture %s: %w", filePath, err)
	}

	fmt.Printf("=== Secrets dechiffres depuis %s (Go) ===\n", filePath)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	inSecrets := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "secrets:" || line == "encrypted_secrets:" {
			inSecrets = true
			continue
		}
		if inSecrets && strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			if strings.HasPrefix(v, "enc:v1:") {
				dec, err := DecryptValue(keyStr, v)
				if err != nil {
					fmt.Printf("%s = [Erreur: %v]\n", k, err)
				} else {
					fmt.Printf("%s = %s\n", k, dec)
				}
			} else if v != "" {
				fmt.Printf("%s = %s (en clair)\n", k, v)
			}
		}
	}
	return nil
}

func main() {
	var (
		keyFlag     = flag.String("key", os.Getenv("NEXTERWAY_TENANT_KEY"), "Cle du tenant (ou variable NEXTERWAY_TENANT_KEY)")
		encryptFlag = flag.String("encrypt", "", "Chiffre une valeur brute")
		decryptFlag = flag.String("decrypt", "", "Dechiffre un payload enc:v1:...")
		setKeyFlag  = flag.String("set-key", "", "Nom de la cle a enregistrer dans le fichier YAML")
		setValFlag  = flag.String("set-val", "", "Valeur en clair de la cle a chiffrer")
		fileFlag    = flag.String("file", "secrets.enc.yaml", "Chemin du fichier secrets chiffres")
		viewFlag    = flag.Bool("view", false, "Dechiffre et affiche l'ensemble des cles du fichier")
	)
	flag.Parse()

	if *keyFlag == "" {
		fmt.Fprintln(os.Stderr, "[Erreur] La cle de chiffrement est obligatoire (-key ou NEXTERWAY_TENANT_KEY).")
		os.Exit(1)
	}

	if *encryptFlag != "" {
		enc, err := EncryptValue(*keyFlag, *encryptFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[Erreur] %v\n", err)
			os.Exit(1)
		}
		fmt.Println(enc)
		return
	}

	if *decryptFlag != "" {
		dec, err := DecryptValue(*keyFlag, *decryptFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[Erreur] %v\n", err)
			os.Exit(1)
		}
		fmt.Println(dec)
		return
	}

	if *setKeyFlag != "" && *setValFlag != "" {
		enc, err := EncryptValue(*keyFlag, *setValFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[Erreur] Chiffrement: %v\n", err)
			os.Exit(1)
		}
		if err := updateSecretsFile(*fileFlag, *setKeyFlag, enc); err != nil {
			fmt.Fprintf(os.Stderr, "[Erreur] Ecriture fichier: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[OK] Cle '%s' chiffree et enregistree dans %s\n", *setKeyFlag, *fileFlag)
		fmt.Printf("     Valeur chiffree : %s...\n", enc[:30])
		return
	}

	if *viewFlag {
		if err := viewSecretsFile(*fileFlag, *keyFlag); err != nil {
			fmt.Fprintf(os.Stderr, "[Erreur] %v\n", err)
			os.Exit(1)
		}
		return
	}

	flag.Usage()
}
