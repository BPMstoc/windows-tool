package main

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

const filePath = "passwords.json"
const encryptionKey = "a16byteslongkey!"

func encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher([]byte(encryptionKey))
	if err != nil {
		return "", err
	}

	nonce := make([]byte, 12)
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	ciphertext := aesgcm.Seal(nil, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(append(nonce, ciphertext...)), nil
}

func decrypt(encryptedText string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encryptedText)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher([]byte(encryptionKey))
	if err != nil {
		return "", err
	}

	if len(data) < 12 {
		return "", fmt.Errorf("invalid ciphertext")
	}

	nonce, ciphertext := data[:12], data[12:]
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func loadPasswords() map[string]string {
	file, err := os.Open(filePath)
	if err != nil {

		if os.IsNotExist(err) {
			return make(map[string]string)
		}
		fmt.Println("Error opening file:", err)
		os.Exit(1)
	}
	defer file.Close()

	encryptedPasswords := make(map[string]string)
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&encryptedPasswords); err != nil {
		fmt.Println("Error decoding JSON:", err)
		os.Exit(1)
	}

	passwords := make(map[string]string)
	for website, encrypted := range encryptedPasswords {
		decrypted, err := decrypt(encrypted)
		if err != nil {
			fmt.Println("Error decrypting password for", website, ":", err)
			os.Exit(1)
		}
		passwords[website] = decrypted
	}

	return passwords
}

func savePasswords(passwords map[string]string) {
	encryptedPasswords := make(map[string]string)
	for website, password := range passwords {
		encrypted, err := encrypt(password)
		if err != nil {
			fmt.Println("Error encrypting password for", website, ":", err)
			os.Exit(1)
		}
		encryptedPasswords[website] = encrypted
	}

	file, err := os.Create(filePath)
	if err != nil {
		fmt.Println("Error creating file:", err)
		os.Exit(1)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(&encryptedPasswords); err != nil {
		fmt.Println("Error encoding JSON:", err)
		os.Exit(1)
	}
}

func main() {
	passwords := loadPasswords()

	fmt.Println("Do you want to check, enter, change or remove a password? (Check/Enter/Change/Remove)")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	choice := scanner.Text()

	if choice == "Enter" {
		fmt.Println("Enter the website:")
		scanner.Scan()
		website := scanner.Text()

		fmt.Println("Enter the password:")
		scanner.Scan()
		password := scanner.Text()

		passwords[website] = password
		savePasswords(passwords)
		fmt.Println("Saved:", website, "->", password)

	} else if choice == "Check" {
		fmt.Println("Enter the website:")
		scanner.Scan()
		website := scanner.Text()

		if password, exists := passwords[website]; exists {
			fmt.Println("Password for", website, "is:", password)
		} else {
			fmt.Println("No password found for", website)
		}

	} else if choice == "Change" {
		fmt.Println("Enter the website you want to change the password of:")
		scanner.Scan()
		website := scanner.Text()

		if password, exists := passwords[website]; exists {
			fmt.Println("Enter the new password:")
			scanner.Scan()
			changedPassword := scanner.Text()
			passwords[website] = changedPassword

			fmt.Println("Password", password, "changed to", changedPassword)
			savePasswords(passwords)
		} else {
			fmt.Println("No password found for", website)
		}

	} else if choice == "Remove" {
		fmt.Println("Enter the website you want to remove:")
		scanner.Scan()
		website := scanner.Text()

		if _, exists := passwords[website]; exists {
			delete(passwords, website)
			fmt.Println("Password for", website, "has been removed.")
			savePasswords(passwords)
		} else {
			fmt.Println("No password found for", website)
		}
		
	} else {
		fmt.Println("Invalid choice. Please enter 'Enter', 'Check', 'Change' or 'Remove'.")
	}
}
