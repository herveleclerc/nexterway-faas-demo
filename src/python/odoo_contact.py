import os
import uuid
from datetime import datetime, timezone
from flask import request, jsonify


PUBLIC_WEBMAILS = {
    "gmail.com", "yahoo.com", "hotmail.com", "outlook.com",
    "orange.fr", "free.fr", "wanadoo.fr", "sfr.fr"
}


def analyze_contact(partner_id, name, email, phone):
    """Effectue un traitement et un enrichissement sur le contact reçu depuis Odoo."""
    email_clean = (email or "").strip().lower()
    name_clean = (name or "").strip()
    phone_clean = (phone or "").strip()

    # Analyse du domaine email
    domain = email_clean.split("@")[-1] if "@" in email_clean else None
    email_type = "invalid"
    if domain:
        email_type = "webmail" if domain in PUBLIC_WEBMAILS else "corporate"

    # Calcul d'un score de complétude (sur 100)
    score = 0
    missing_fields = []
    if name_clean:
        score += 30
    else:
        missing_fields.append("name")

    if email_clean and domain:
        score += 40
    else:
        missing_fields.append("email")

    if phone_clean:
        score += 30
    else:
        missing_fields.append("phone")

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
    """Point d'entree FaaS Fission adapté au format Webhook d'Odoo (POST /api/odoo)."""
    payload = request.get_json(silent=True)
    if not payload:
        return jsonify({
            "status": "error",
            "code": 400,
            "message": "Corps de requete JSON invalide ou absent"
        }), 400

    # --- AJUSTEMENT WEBHOOK ODOO NATIF ---
    # Le Webhook Odoo encapsule l'objet dans une liste sous la clé 'data'
    records = payload.get("data", [])
    if not records:
        return jsonify({
            "status": "error",
            "code": 400,
            "message": "Aucune donnée trouvée dans la clé 'data' du Webhook Odoo"
        }), 400
    
    # On extrait le premier enregistrement envoyé (john doe)
    contact_data = records[0]

    # Le Webhook natif utilise directement la clé 'id' pour l'ID de la table
    partner_id = contact_data.get("id")
    name = contact_data.get("name")
    email = contact_data.get("email")
    phone = contact_data.get("phone")
    # -------------------------------------

    # Traitement métier & enrichissement (inchangé)
    contact_analysis = analyze_contact(partner_id, name, email, phone)

    tracking_id = f"odoo-sync-{uuid.uuid4().hex[:12]}"
    now = datetime.now(timezone.utc).isoformat()

    return jsonify({
        "status": "success",
        "code": 200,
        "message": f"Contact Odoo #{partner_id} ({name}) traite avec succes par FaaS Nexterway",
        "tracking_id": tracking_id,
        "received_at": now,
        "processed_by_pod": os.getenv("HOSTNAME", "local"),
        "contact": contact_analysis
    }), 200
