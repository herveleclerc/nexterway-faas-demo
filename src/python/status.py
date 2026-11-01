import os
import platform
import time
from datetime import datetime, timezone
import psutil
import requests


def check_upstream_connectivity(url="https://api.github.com/zen", timeout=2.0):
    """Vérifie la connectivité sortante vers un service distant."""
    try:
        start = time.perf_counter()
        response = requests.get(url, timeout=timeout)
        duration_ms = round((time.perf_counter() - start) * 1000, 2)
        return {
            "status": "reachable" if response.ok else f"http_{response.status_code}",
            "latency_ms": duration_ms,
        }
    except requests.RequestException as err:
        return {
            "status": "unreachable",
            "error": str(err),
        }


def main():
    # Métriques système du pod (via psutil)
    virtual_mem = psutil.virtual_memory()
    memory_info = {
        "total_mb": round(virtual_mem.total / (1024 * 1024), 2),
        "available_mb": round(virtual_mem.available / (1024 * 1024), 2),
        "percent_used": virtual_mem.percent,
    }

    cpu_info = {
        "count": psutil.cpu_count(logical=True),
        "percent": psutil.cpu_percent(interval=None),
    }

    # Test de connectivité réseau sortante (via requests)
    upstream = check_upstream_connectivity()

    # Fission sérialise directement les dictionnaires Python en JSON
    return {
        "status": "healthy" if upstream["status"] == "reachable" else "degraded",
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "pod": {
            "name": os.getenv("HOSTNAME", "local"),
            "python_version": platform.python_version(),
            "os": platform.platform(),
        },
        "system": {
            "cpu": cpu_info,
            "memory": memory_info,
        },
        "upstream": upstream,
    }
