import os
import json
import urllib.request
from flask import request, jsonify

INTERNAL_SERVICE_TOKEN = os.getenv("INTERNAL_SERVICE_TOKEN", "nexterway-internal-token-xyz")
ROUTER_URL = os.getenv("FISSION_ROUTER_URL", "http://router.fission")
TENANT_HOST = os.getenv("TENANT_HOST", "wy-3a1fd37f-1-0-0.nexterway.caas.fr")


def main():
    """Orchestrateur public : appelle en interne le service securise."""
    qty = request.args.get("quantity", "7")

    # Appel interne vers la fonction internal-compute via le routeur Fission
    url = f"{ROUTER_URL}/api/internal-compute?value={qty}"
    req = urllib.request.Request(url, headers={
        "Host": TENANT_HOST,
        "X-Internal-Token": INTERNAL_SERVICE_TOKEN
    })

    try:
        with urllib.request.urlopen(req, timeout=10) as response:
            calc_data = json.loads(response.read().decode("utf-8"))
    except Exception as e:
        return jsonify({
            "status": "error",
            "message": f"Echec de la communication avec le service interne: {str(e)}"
        }), 502

    return jsonify({
        "status": "success",
        "order_id": "ORD-98432",
        "orchestrator_pod": os.getenv("HOSTNAME", "local"),
        "summary": "Commande traitee avec calcul interne securise",
        "computation": calc_data
    }), 200
