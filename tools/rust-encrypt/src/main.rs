use std::env;
use std::fs;
use std::io::{self, BufRead};
use std::path::Path;
use std::process;

use aes_gcm::aead::{Aead, KeyInit};
use aes_gcm::{Aes256Gcm, Nonce};
use base64::engine::general_purpose::STANDARD as BASE64;
use base64::Engine;
use rand::RngCore;

fn get_cipher_key(key_str: &str) -> [u8; 32] {
    let mut key = [0u8; 32];
    let bytes = key_str.as_bytes();
    let len = bytes.len().min(32);
    key[..len].copy_from_slice(&bytes[..len]);
    key
}

fn encrypt(key_str: &str, plaintext: &str) -> Result<String, String> {
    let key = get_cipher_key(key_str);
    let cipher = Aes256Gcm::new_from_slice(&key).map_err(|e| e.to_string())?;

    let mut nonce_bytes = [0u8; 12];
    rand::thread_rng().fill_bytes(&mut nonce_bytes);
    let nonce = Nonce::from_slice(&nonce_bytes);

    let ciphertext = cipher
        .encrypt(nonce, plaintext.as_bytes())
        .map_err(|e| e.to_string())?;

    let mut payload = Vec::with_capacity(12 + ciphertext.len());
    payload.extend_from_slice(&nonce_bytes);
    payload.extend_from_slice(&ciphertext);

    Ok(format!("enc:v1:{}", BASE64.encode(payload)))
}

fn decrypt(key_str: &str, payload_str: &str) -> Result<String, String> {
    if !payload_str.starts_with("enc:v1:") {
        return Ok(payload_str.to_string());
    }

    let raw = BASE64
        .decode(&payload_str[7..])
        .map_err(|e| e.to_string())?;

    if raw.len() < 28 {
        return Err("Payload trop court (nonce 12 octets + tag 16 octets requis)".to_string());
    }

    let (nonce_bytes, ciphertext) = raw.split_at(12);
    let key = get_cipher_key(key_str);
    let cipher = Aes256Gcm::new_from_slice(&key).map_err(|e| e.to_string())?;
    let nonce = Nonce::from_slice(nonce_bytes);

    let plaintext_bytes = cipher
        .decrypt(nonce, ciphertext)
        .map_err(|e| format!("Echec dechiffrement: {}", e))?;

    String::from_utf8(plaintext_bytes).map_err(|e| e.to_string())
}

fn update_secrets_file(file_path: &str, key_name: &str, encrypted_val: &str) -> io::Result<()> {
    let mut lines = Vec::new();
    let mut found_key = false;
    let mut in_secrets = false;

    if Path::new(file_path).is_file() {
        let content = fs::read_to_string(file_path)?;
        for line in content.lines() {
            let trimmed = line.trim();
            if trimmed.starts_with("secrets:") || trimmed.starts_with("encrypted_secrets:") {
                in_secrets = true;
                lines.push(line.to_string());
                continue;
            }

            if in_secrets && trimmed.starts_with(&format!("{}:", key_name)) {
                lines.push(format!("  {}: {}", key_name, encrypted_val));
                found_key = true;
                continue;
            }

            lines.push(line.to_string());
        }
    } else {
        lines.push("version: 1".to_string());
        lines.push("secrets:".to_string());
        in_secrets = true;
    }

    if !found_key {
        if !in_secrets {
            lines.push("secrets:".to_string());
        }
        lines.push(format!("  {}: {}", key_name, encrypted_val));
    }

    let output = lines.join("\n") + "\n";
    fs::write(file_path, output)
}

fn view_secrets_file(file_path: &str, key_str: &str) -> io::Result<()> {
    let file = fs::File::open(file_path)?;
    let reader = io::BufReader::new(file);

    println!("=== Secrets dechiffres depuis {} (Rust) ===", file_path);
    let mut in_secrets = false;

    for line in reader.lines() {
        let l = line?;
        let trimmed = l.trim();
        if trimmed == "secrets:" || trimmed == "encrypted_secrets:" {
            in_secrets = true;
            continue;
        }

        if in_secrets && trimmed.contains(':') {
            let parts: Vec<&str> = trimmed.splitn(2, ':').collect();
            let k = parts[0].trim();
            let v = parts[1].trim();

            if v.starts_with("enc:v1:") {
                match decrypt(key_str, v) {
                    Ok(dec) => println!("{} = {}", k, dec),
                    Err(e) => println!("{} = [Erreur: {}]", k, e),
                }
            } else if !v.is_empty() {
                println!("{} = {} (en clair)", k, v);
            }
        }
    }

    Ok(())
}

fn print_help() {
    println!("Usage: rust-encrypt [OPTIONS]");
    println!();
    println!("Options:");
    println!("  --key <CLE>         Cle du tenant (ou variable NEXTERWAY_TENANT_KEY)");
    println!("  --encrypt <VALEUR>  Chiffre une valeur brute");
    println!("  --decrypt <PAYLOAD> Dechiffre un payload enc:v1:...");
    println!("  --set <CLE> <VAL>   Chiffre et enregistre la cle dans secrets.enc.yaml");
    println!("  --view              Affiche tous les secrets dechiffres du fichier");
    println!("  --file <PATH>       Chemin du fichier (defaut: secrets.enc.yaml)");
    println!("  --help              Affiche cette aide");
}

fn main() {
    let args: Vec<String> = env::args().collect();
    let mut key = env::var("NEXTERWAY_TENANT_KEY").unwrap_or_default();
    let mut file = "secrets.enc.yaml".to_string();

    let mut encrypt_val = None;
    let mut decrypt_val = None;
    let mut set_pair = None;
    let mut view_flag = false;

    let mut i = 1;
    while i < args.len() {
        match args[i].as_str() {
            "--key" => {
                if i + 1 < args.len() {
                    key = args[i + 1].clone();
                    i += 1;
                }
            }
            "--file" => {
                if i + 1 < args.len() {
                    file = args[i + 1].clone();
                    i += 1;
                }
            }
            "--encrypt" => {
                if i + 1 < args.len() {
                    encrypt_val = Some(args[i + 1].clone());
                    i += 1;
                }
            }
            "--decrypt" => {
                if i + 1 < args.len() {
                    decrypt_val = Some(args[i + 1].clone());
                    i += 1;
                }
            }
            "--set" => {
                if i + 2 < args.len() {
                    set_pair = Some((args[i + 1].clone(), args[i + 2].clone()));
                    i += 2;
                }
            }
            "--view" => {
                view_flag = true;
            }
            "--help" | "-h" => {
                print_help();
                return;
            }
            _ => {}
        }
        i += 1;
    }

    if key.is_empty() {
        eprintln!("[Erreur] La cle est obligatoire (--key ou NEXTERWAY_TENANT_KEY)");
        process::exit(1);
    }

    if let Some(val) = encrypt_val {
        match encrypt(&key, &val) {
            Ok(enc) => println!("{}", enc),
            Err(e) => {
                eprintln!("[Erreur] {}", e);
                process::exit(1);
            }
        }
        return;
    }

    if let Some(val) = decrypt_val {
        match decrypt(&key, &val) {
            Ok(dec) => println!("{}", dec),
            Err(e) => {
                eprintln!("[Erreur] {}", e);
                process::exit(1);
            }
        }
        return;
    }

    if let Some((k, v)) = set_pair {
        match encrypt(&key, &v) {
            Ok(enc) => {
                if let Err(e) = update_secrets_file(&file, &k, &enc) {
                    eprintln!("[Erreur fichier] {}", e);
                    process::exit(1);
                }
                println!("[OK] Cle '{}' chiffree et enregistree dans {}", k, file);
                println!("     Valeur chiffree : {}...", &enc[..30.min(enc.len())]);
            }
            Err(e) => {
                eprintln!("[Erreur chiffrement] {}", e);
                process::exit(1);
            }
        }
        return;
    }

    if view_flag {
        if let Err(e) = view_secrets_file(&file, &key) {
            eprintln!("[Erreur] {}", e);
            process::exit(1);
        }
        return;
    }

    print_help();
}

