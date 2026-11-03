#!/usr/bin/env node
/**
 * Utilitaire de chiffrement de secrets en Node.js pour Nexterway FaaS.
 * Format standard compatible : AES-256-GCM.
 */

const crypto = require('crypto');
const fs = require('fs');

function getCipherKey(keyStr) {
  const key = Buffer.alloc(32, 0);
  Buffer.from(keyStr, 'utf8').copy(key, 0, 0, 32);
  return key;
}

function encrypt(keyStr, plaintext) {
  const key = getCipherKey(keyStr);
  const iv = crypto.randomBytes(12);
  const cipher = crypto.createCipheriv('aes-256-gcm', key, iv);
  const enc = Buffer.concat([cipher.update(plaintext, 'utf8'), cipher.final()]);
  const tag = cipher.getAuthTag();
  const payload = Buffer.concat([iv, enc, tag]);
  return 'enc:v1:' + payload.toString('base64');
}

function decrypt(keyStr, payloadBase64) {
  if (!payloadBase64.startsWith('enc:v1:')) return payloadBase64;
  const raw = Buffer.from(payloadBase64.slice(7), 'base64');
  if (raw.length < 28) throw new Error('Payload trop court');
  const iv = raw.subarray(0, 12);
  const tag = raw.subarray(raw.length - 16);
  const ciphertext = raw.subarray(12, raw.length - 16);

  const key = getCipherKey(keyStr);
  const decipher = crypto.createDecipheriv('aes-256-gcm', key, iv);
  decipher.setAuthTag(tag);
  const decrypted = Buffer.concat([decipher.update(ciphertext), decipher.final()]);
  return decrypted.toString('utf8');
}

const args = process.argv.slice(2);
const key = process.env.NEXTERWAY_TENANT_KEY || (args.includes('--key') ? args[args.indexOf('--key') + 1] : null);

if (!key) {
  console.error('[Erreur] Specifiez la cle de chiffrement via --key <CLE> ou NEXTERWAY_TENANT_KEY');
  process.exit(1);
}

if (args.includes('--encrypt')) {
  const val = args[args.indexOf('--encrypt') + 1];
  console.log(encrypt(key, val));
} else if (args.includes('--decrypt')) {
  const val = args[args.indexOf('--decrypt') + 1];
  console.log(decrypt(key, val));
} else {
  console.log('Usage: node tools/encrypt.js --key <CLE> [--encrypt <VALEUR> | --decrypt <PAYLOAD>]');
}

