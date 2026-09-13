package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/FFReStart/FFReStart-Launcher/internal/patch"
	"github.com/FFReStart/FFReStart-Launcher/internal/update"
)

func main() {
	if err := run(os.Args[1:], os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, getenv func(string) string) error {
	if len(args) == 0 || (args[0] != "sign" && args[0] != "verify") {
		return errors.New("usage: signmanifest <sign|verify> -type <launcher|game> -manifest <path> (-key-env <name>|-key-file <path>)")
	}
	mode := args[0]
	flags := flag.NewFlagSet("signmanifest "+mode, flag.ContinueOnError)
	manifestType := flags.String("type", "", "launcher or game")
	manifestPath := flags.String("manifest", "", "manifest path")
	outputPath := flags.String("output", "", "signed manifest output path")
	keyEnvironment := flags.String("key-env", "", "environment variable containing key material")
	keyPath := flags.String("key-file", "", "key file path")
	development := flags.Bool("development", false, "allow test keys and plain HTTP")
	targetOS := flags.String("target-os", runtime.GOOS, "game manifest target OS")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if *manifestPath == "" || (*manifestType != "launcher" && *manifestType != "game") || (*keyEnvironment == "") == (*keyPath == "") {
		return errors.New("manifest type, manifest path, and exactly one key source are required")
	}
	keyData, err := readKey(*keyEnvironment, *keyPath, getenv)
	if err != nil {
		return err
	}
	manifestData, err := os.ReadFile(*manifestPath) // #nosec G304,G703 -- caller explicitly supplies the manifest.
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	if mode == "sign" {
		if *outputPath == "" {
			return errors.New("sign mode requires -output")
		}
		privateKey, err := decodePrivateKey(keyData)
		if err != nil {
			return err
		}
		return sign(*manifestType, *targetOS, manifestData, privateKey, !*development, *outputPath)
	}
	publicKey, err := decodePublicKey(keyData)
	if err != nil {
		return err
	}
	return verify(*manifestType, *targetOS, manifestData, publicKey, !*development)
}

func readKey(environment, path string, getenv func(string) string) ([]byte, error) {
	if environment != "" {
		value := getenv(environment)
		if strings.TrimSpace(value) == "" {
			return nil, errors.New("signing key environment variable is absent")
		}
		return []byte(value), nil
	}
	value, err := os.ReadFile(path) // #nosec G304,G703 -- caller explicitly supplies the key file.
	if err != nil {
		return nil, errors.New("read signing key file")
	}
	return value, nil
}

func decodeKey(data []byte) ([]byte, error) {
	encoded := strings.TrimSpace(string(data))
	if decoded, err := hex.DecodeString(encoded); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(encoded); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.StdEncoding.DecodeString(encoded); err == nil {
		return decoded, nil
	}
	if len(data) == ed25519.SeedSize || len(data) == ed25519.PrivateKeySize {
		return bytes.Clone(data), nil
	}
	return nil, errors.New("signing key must be raw, hex, or base64 Ed25519 material")
}

func decodePrivateKey(data []byte) (ed25519.PrivateKey, error) {
	decoded, err := decodeKey(data)
	if err != nil {
		return nil, err
	}
	switch len(decoded) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(decoded), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(bytes.Clone(decoded)), nil
	default:
		return nil, errors.New("signing key has an invalid Ed25519 private-key length")
	}
}

func decodePublicKey(data []byte) (ed25519.PublicKey, error) {
	decoded, err := decodeKey(data)
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		return nil, errors.New("verification key has an invalid Ed25519 public-key length")
	}
	return ed25519.PublicKey(decoded), nil
}

func sign(manifestType, targetOS string, data []byte, privateKey ed25519.PrivateKey, production bool, output string) error {
	publicKey := privateKey.Public().(ed25519.PublicKey)
	var result any
	switch manifestType {
	case "launcher":
		manifest, err := update.ParseUnsignedManifestWithPolicy(data, !production)
		if err != nil {
			return err
		}
		if production {
			if _, err := update.ReleasePublicKey(manifest.KeyID, hex.EncodeToString(publicKey), true); err != nil {
				return err
			}
		}
		manifest.Signature = base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, manifest.SigningBytes()))
		result = manifest
	case "game":
		manifest, err := patch.ParseUnsignedManifestForTarget(data, targetOS, !production)
		if err != nil {
			return err
		}
		if production {
			if _, err := update.ReleasePublicKey(manifest.KeyID, hex.EncodeToString(publicKey), true); err != nil {
				return err
			}
		}
		signed, err := manifest.SigningBytes()
		if err != nil {
			return err
		}
		manifest.Signature = base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, signed))
		result = manifest
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(output, encoded, 0o600); err != nil { // #nosec G306,G703 -- manifest is a public release artifact.
		return fmt.Errorf("write signed manifest: %w", err)
	}
	return nil
}

func verify(manifestType, targetOS string, data []byte, publicKey ed25519.PublicKey, production bool) error {
	switch manifestType {
	case "launcher":
		manifest, err := update.ParseManifestWithPolicy(data, !production)
		if err != nil {
			return err
		}
		return update.VerifySignature(manifest, manifest.KeyID, publicKey)
	case "game":
		manifest, err := patch.ParseManifestForTarget(data, targetOS, !production)
		if err != nil {
			return err
		}
		return patch.VerifyManifest(manifest, manifest.KeyID, publicKey)
	default:
		return errors.New("unsupported manifest type")
	}
}
