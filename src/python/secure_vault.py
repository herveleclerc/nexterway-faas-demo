import os
from flask import request, jsonify

EXPECTED_API_KEY = os.getenv("API_KEY", "nexterway-secret-key-2026")


def main():
    """Fonction protegee par cle d'API externe (X-API-Key ou Bearer token)."""
    auth_header = request.headers.get("X-API-Key")
    if not auth_header:
        bearer = request.headers.get("Authorization", "")
        if bearer.startswith("Bearer "):
            auth_header = bearer[7:].strip()

    if auth_header != EXPECTED_API_KEY:
        return jsonify({
            "status": "error",
            "code": 401,
            "message": "Acces refuse : cle API invalide ou absente (entete X-API-Key requis)"
        }), 401

    return jsonify({
        "status": "success",
        "code": 200,
        "message": "Acces autorise au coffre securise Nexterway FaaS",
        "data": {
            "vault_item": "top-secret-credentials-alpha",
            "environment": os.getenv("ENVIRONMENT", "production"),
            "caller_ip": request.remote_addr,
            "method": request.method
        }
    }), 200
