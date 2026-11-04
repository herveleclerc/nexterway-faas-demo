import logging
import requests
from odoo import models, api
from odoo.exceptions import UserError

_logger = logging.getLogger(__name__)

class ResPartner(models.Model):
    _inherit = 'res.partner'

    def write(self, vals):
        res = super(ResPartner, self).write(vals)
        if any(field in vals for field in ['name', 'phone', 'email']):
            for record in self:
                if not record.parent_id:
                    url = "https://wy-3a1fd37f-1-0-0.nexterway.caas.fr/api/odoo"
                    payload = {
                        "id": record.id,
                        "name": record.name,
                        "email": record.email or "",
                        "phone": record.phone or ""
                    }
                    try:
                        # L'appel s'exécute ici au niveau du serveur, hors du bac à sable
                        response = requests.post(url, json=payload, timeout=10)
                        if response.status_code == 200:
                            result = response.json()
                            contact_analysis = result.get('contact', {})
                            score = contact_analysis.get('profile_completeness_pct', 0)
                            action_req = contact_analysis.get('action_required', '')

                            # Écriture directe du résultat
                            record.comment = (
                                f"--- ANALYSE FAAS (MODULE ZIP) ---\n"
                                f"Score de complétude : {score}%\n"
                                f"Statut : {action_req}\n"
                                f"Tracking ID : {result.get('tracking_id')}"
                            )
                    except Exception as e:
                        _logger.error(f"Erreur FaaS : {e}")
        return res
