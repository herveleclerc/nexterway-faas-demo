#!/usr/bin/env python3
"""
Utilitaire de chiffrement de secrets pour Nexterway FaaS (GitOps).
Permet de chiffrer des variables sensibles au format AES-256-GCM
avec la cle privee du tenant recuperee depuis le portail Nexterway.
"""

import sys
import os
import argparse
import base64
from pathlib import Path

try:
    from cryptography.hazmat.primitives.ciphers.aead import AESGCM
except ImportError:
    print("[Erreur] La bibliotheque 'cryptography' est requise.")
    print("Installez-la via: pip install cryptography")
    sys.exit(1)

try:
    import yaml
except ImportError:
    yaml = None


def get_cipher(key_str: str) -> AESGCM:
    key_bytes = key_str.encode("utf-8")[:32].ljust(32, b"0")
    return AESGCM(key_bytes)


def encrypt_value(key_str: str, plaintext: str) -> str:
    """Chiffre une chaine en AES-256-GCM avec nonce de 12 octets."""
    aesgcm = get_cipher(key_str)
    nonce = os.urandom(12)
    ciphertext = aesgcm.encrypt(nonce, plaintext.encode("utf-8"), None)
    payload = nonce + ciphertext
    return "enc:v1:" + base64.b64encode(payload).decode("utf-8")


def decrypt_value(key_str: str, encrypted_payload: str) -> str:
    """Dechiffre un payload enc:v1:."""
    if not encrypted_payload.startswith("enc:v1:"):
        return encrypted_payload
    raw = base64.b64decode(encrypted_payload[7:])
    if len(raw) < 28:
        raise ValueError("Payload trop court pour contenir le nonce et l'auth tag.")
    nonce = raw[:12]
    ciphertext = raw[12:]
    aesgcm = get_cipher(key_str)
    decrypted = aesgcm.decrypt(nonce, ciphertext, None)
    return decrypted.decode("utf-8")


def main():
    parser = argparse.ArgumentParser(
        description="Outil de gestion des secrets chiffres pour Nexterway FaaS"
    )
    parser.add_argument(
        "--key",
        default=os.environ.get("NEXTERWAY_TENANT_KEY", ""),
        help="Cle du tenant (ou variable NEXTERWAY_TENANT_KEY)",
    )
    parser.add_argument(
        "--encrypt",
        metavar="TEXTE",
        help="Chiffre une valeur brute et l'affiche sur stdout",
    )
    parser.add_argument(
        "--decrypt",
        metavar="PAYLOAD",
        help="Dechiffre une valeur enc:v1:... et l'affiche",
    )
    parser.add_argument(
        "--set",
        nargs=2,
        metavar=("CLE", "VALEUR"),
        help="Ajoute ou met a jour une cle dans le fichier secrets.enc.yaml",
    )
    parser.add_argument(
        "--file",
        default="secrets.enc.yaml",
        help="Chemin du fichier secrets chiffres (defaut: secrets.enc.yaml)",
    )
    parser.add_argument(
        "--view",
        action="store_true",
        help="Dechiffre et affiche l'ensemble des cles du fichier secrets.enc.yaml",
    )

    args = parser.parse_args()

    if not args.key:
        print("[Erreur] La cle de chiffrement est obligatoire (--key ou NEXTERWAY_TENANT_KEY).")
        print("Recuperez votre cle dans le portail Nexterway > Application FaaS > Informations (secrets).")
        sys.exit(1)

    if args.encrypt:
        print(encrypt_value(args.key, args.encrypt))
        return

    if args.decrypt:
        try:
            print(decrypt_value(args.key, args.decrypt))
        except Exception as e:
            print(f"[Erreur de dechiffrement] {e}")
            sys.exit(1)
        return

    if args.set:
        key_name, plaintext = args.set
        encrypted_val = encrypt_value(args.key, plaintext)

        secrets_dict = {}
        file_path = Path(args.file)
        if file_path.is_file():
            if yaml:
                with open(file_path, "r", encoding="utf-8") as f:
                    content = yaml.safe_load(f) or {}
                secrets_dict = content.get("secrets", content.get("encrypted_secrets", content))
                if not isinstance(secrets_dict, dict):
                    secrets_dict = {}
            else:
                print("[Avertissement] PyYAML non detecte, creation d'un nouveau fichier.")

        secrets_dict[key_name] = encrypted_val

        output_data = {
            "version": 1,
            "secrets": secrets_dict,
        }

        with open(file_path, "w", encoding="utf-8") as f:
            if yaml:
                yaml.dump(output_data, f, default_flow_style=False, sort_keys=False)
            else:
                import json
                f.write(json.dumps(output_data, indent=2))

        print(f"[OK] Cle '{key_name}' chiffree et enregistree dans {args.file}")
        print(f"     Valeur chiffree : {encrypted_val[:30]}...")
        return

    if args.view:
        file_path = Path(args.file)
        if not file_path.is_file():
            print(f"[Erreur] Fichier {args.file} introuvable.")
            sys.exit(1)

        if not yaml:
            print("[Erreur] PyYAML est requis pour --view. Installez: pip install pyyaml")
            sys.exit(1)

        with open(file_path, "r", encoding="utf-8") as f:
            content = yaml.safe_load(f) or {}
        items = content.get("secrets", content.get("encrypted_secrets", content))

        print(f"=== Secrets dechiffres depuis {args.file} ===")
        for k, v in items.items():
            if isinstance(v, str) and v.startswith("enc:v1:"):
                try:
                    dec = decrypt_value(args.key, v)
                    print(f"{k} = {dec}")
                except Exception as e:
                    print(f"{k} = [Erreur: {e}]")
            else:
                print(f"{k} = {v} (en clair)")
        return

    parser.print_help()


if __name__ == "__main__":
    main()
