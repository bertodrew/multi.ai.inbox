# FASE 2 — Secondo shared node (provisioning pipeline, non autoscaling) — AImulti.inbox
#
# Design: landing/docs/iaas-design.md §1-§2 — capacity planning + provisioning
# pipeline: lo scale UP è provisioning del prossimo nodo su metriche pre-allarme
# (allarme a 70% del cap nodo, provisioning a 85%; cap 80 tenant/nodo), MAI
# reattivo sul load istantaneo. Lo scale DOWN è consolidamento pianificato.
#
# File FASE 2: non tocca i file del modulo Fase 1 (main.tf, variables.tf, install.sh, README.md).
# ATTENZIONE: nessun `terraform apply` in automatico — solo init/plan revisionato.
# Runbook operativo: landing/infra/LB.md

# ---------------------------------------------------------------------------
# Variabili
# ---------------------------------------------------------------------------

variable "node_count" {
  description = "Shared node da creare. Il nodo 1 esiste già (server attuale, target IP 2.28.99.252): Terraform crea i nodi da 2 in su."
  type        = number
  default     = 1

  validation {
    condition     = var.node_count >= 0 && var.node_count <= 10
    error_message = "node_count tra 0 e 10: lo scale è provisioning pianificato, non autoscaling."
  }
}

variable "node_name_prefix" {
  description = "Prefisso nome server Hetzner (numerazione da 2: il nodo 1 esiste già)"
  type        = string
  default     = "aimulti-shared"
}

# AX41-NVMe (€59/mo, 64 GB, 2×512 GB NVMe RAID1) è un DEDICATED ROOT SERVER
# Robot: NON è un tipo hcloud. Per usarlo: ordine via Robot + IP aggiunto a
# var.target_ips (lb.tf), oppure provider hrobot. Per il provisioning gestito
# hcloud usare un tipo CX/CPX/CCX (default CX42: 8 vCPU dedicati / 16 GB).
variable "server_type" {
  description = "Tipo server hcloud per i nodi shared (AX41-NVMe = Robot, vedi commento sopra)"
  type        = string
  default     = "CX42"
}

variable "node_image" {
  description = "Immagine dei nodi (Debian 12: base della procedura install già validata su Hetzner)"
  type        = string
  default     = "debian-12"
}

variable "node_location" {
  description = "Location Hetzner dei nodi shared (stessa del LB)"
  type        = string
  default     = "fsn1"
}

variable "ssh_keys" {
  description = "ID o nome delle chiavi SSH hcloud per i nodi (dalla pipeline Fase 1). Default vuoto: passarle PRIMA dell'apply, un nodo senza chiavi è irraggiungibile."
  type        = list(string)
  default     = []
}

# ---------------------------------------------------------------------------
# Shared node — HTTP via LB (lb.tf), SMTP via DNS MX, nessun LB interno
# ---------------------------------------------------------------------------

resource "hcloud_server" "shared" {
  for_each    = { for i in range(var.node_count) : tostring(i) => i }
  name        = "${var.node_name_prefix}-${each.value + 2}"
  server_type = var.server_type
  image       = var.node_image
  location    = var.node_location
  ssh_keys    = var.ssh_keys

  labels = {
    product = "aimulti-inbox"
    role    = "shared-mail-node"
  }

  # Cloud-init: Docker + bootstrap stack. Finché install.sh non gira, il nodo
  # non risponde a /landingpage/api/status → il LB lo tiene FUORI rotazione
  # (health check) e SMTP non lo usa finché non c'è l'A record mail2.
  user_data = <<-CLOUDINIT
    #cloud-config
    package_update: true
    packages:
      - ca-certificates
      - curl
      - git
    runcmd:
      - 'curl -fsSL https://get.docker.com | sh'
      - 'install -d -m 0755 /opt/aimulti-inbox'
      - 'if [ -f /opt/aimulti-inbox/install.sh ]; then bash /opt/aimulti-inbox/install.sh > /var/log/aimulti-install.log 2>&1; else echo "install.sh non trovato in /opt/aimulti-inbox: prerequisito provisioning manuale, vedi landing/infra/LB.md sezione Prerequisito nodo" > /opt/aimulti-inbox/INSTALL_REQUIRED; fi'
      - 'systemctl enable --now docker'
    final_message: "bootstrap AImulti.inbox completato: docker pronto, stack via install.sh (assente = INSTALL_REQUIRED, nodo resta fuori rotazione LB)"
  CLOUDINIT

  # Nodo STATEFUL (maildir, MySQL, Redis, code postfix): mai rebuild per un
  # cambio user_data — le modifiche al bootstrap vanno su nodi NUOVI
  # (provisioning), non su quelli in produzione.
  lifecycle {
    ignore_changes = [user_data]
  }
}

# ---------------------------------------------------------------------------
# Output
# ---------------------------------------------------------------------------

output "shared_node_ipv4" {
  description = "IPv4 dei shared node creati (da accodare a var.target_ips in lb.tf per HTTP, e usati per l'A record mail2)"
  value       = [for s in hcloud_server.shared : s.ipv4_address]
}
