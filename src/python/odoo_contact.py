import os
import uuid
from datetime import datetime, timezone
from flask import request, jsonify, current_app # Ajout de current_app

PUBLIC_WEBMAILS = {
    "gmail.com", "yahoo.com", "hotmail.com", "outlook.com",
    "orange.fr", "free.fr", "wanadoo.fr", "sfr.fr"
}

def analyze_contact(partner_id, name, email, phone):
    """Traitement et enrichissement sur le contact reçu depuis Odoo 19."""
    email_clean = (email or "").strip().lower() if email else ""
    name_clean = (name or "").strip()
    phone_clean = (phone or "").strip() if phone else ""

    domain = email_clean.split("@")[-1] if "@" in email_clean else None
    email_type = "invalid"
    if domain:
        email_type = "webmail" if domain in PUBLIC_WEBMAILS else "corporate"

    score = 0
    missing_fields = []
    if name_clean: score += 30
    else: missing_fields.append("name")

    if email_clean and domain: score += 40
    else: missing_fields.append("email")

    if phone_clean: score += 30
    else: missing_fields.append("phone")

    return {
        "partner_id": partner_id,
        "name": name_clean,
        "email": email_clean,
        "phone": phone_clean,
        "email_domain": domain,
        "contact_type": email_type,
        "profile_completeness_pct": score,
        "missing_fields": missing_fields,
        "action_required": "complete_profile" if score < 70 else "ready_for_sync"
    }

def main():
    """Point d'entree FaaS Fission adapté au format direct d'Odoo 19 avec Logs Forcés."""
    
    # LOG DE DÉMARRAGE : S'affiche dès que la fonction reçoit le moindre octet
    current_app.logger.info("=== [FAAS] REQUETE RECUE DEPUIS ODOO ===")

    payload = request.get_json(silent=True)
    if not payload:
        current_app.logger.error("[FAAS] ECHEC : Le corps de la requête JSON est vide ou invalide")
        return jsonify({"status": "error", "code": 400, "message": "JSON manquant"}), 400

    # LOG DU PAYLOAD : Pour voir exactement ce qu'Odoo a envoyé
    current_app.logger.info(f"[FAAS] DONNEES BRUTES REÇUES : {payload}")

    partner_id = payload.get("id")
    name = payload.get("name")
    
    email = payload.get("email")
    if email is False or email is None: email = ""
        
    phone = payload.get("phone")
    if phone is False or phone is None: phone = ""

    # Exécution de la logique métier
    contact_analysis = analyze_contact(partner_id, name, email, phone)

    # LOG DE SUCCÈS : Affiche le résultat de l'analyse dans votre terminal
    current_app.logger.info(f"[FAAS] TRAITEMENT SUCCES -> Contact: {name} (ID: {partner_id}) - Score: {contact_analysis['profile_completeness_pct']}%")

    tracking_id = f"odoo-sync-{uuid.uuid4().hex[:12]}"
    now = datetime.now(timezone.utc).isoformat()

    return jsonify({
        "status": "success",
        "code": 200,
        "message": f"Contact Odoo #{partner_id} ({name}) traite avec succes par FaaS",
        "tracking_id": tracking_id,
        "received_at": now,
        "processed_by_pod": os.getenv("HOSTNAME", "local"),
        "contact": contact_analysis
    }), 200
