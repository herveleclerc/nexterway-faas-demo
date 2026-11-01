from flask import request, jsonify
def main():
    return jsonify({"message": "Hello from Fission!", "status": "ok"})