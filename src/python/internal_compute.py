import os
from flask import request, jsonify

INTERNAL_SERVICE_TOKEN = os.getenv("INTERNAL_SERVICE_TOKEN", "nexterway-internal-token-xyz")


def main():
    """Service interne : exige obligatoirement l'entete X-Internal-Token."""
    token = request.headers.get("X-Internal-Token")
    if token != INTERNAL_SERVICE_TOKEN:
        return jsonify({
            "status": "error",
            "code": 401,
            "message": "Rejet : service interne non autorise. Entete X-Internal-Token manquant ou invalide."
        }), 401

    val = request.args.get("value", "10")
    try:
        num = int(val)
    except ValueError:
        num = 10

    result = {
        "service": "internal-compute",
        "status": "computed",
        "input_value": num,
        "square": num * num,
        "cube": num * num * num,
        "processed_by_pod": os.getenv("HOSTNAME", "unknown-pod")
    }
    return jsonify(result), 200

