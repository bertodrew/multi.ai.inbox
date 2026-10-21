# FASE 2 — Load balancing HTTP (Hetzner LB) + DNS MX multipli (SMTP) — AImulti.inbox
#
# Design: landing/docs/iaas-design.md §3 — LB managed per HTTP (landing/API/webmail),
# DNS MX per SMTP (la mail è nativamente distribuita: nessun LB device, il DNS fa
# il bilanciamento, failover gratis con priorità).
# Vendor: Hetzner (DE) per il LB + Namecheap (registrar/DNS della zone). Nessun US hyperscaler.
#
# File FASE 2: non tocca i file del modulo Fase 1 (main.tf, variables.tf, install.sh, README.md).
# ATTENZIONE: nessun `terraform apply` in automatico — solo init/plan revisionato.
# Runbook operativo: landing/infra/LB.md

terraform {
  required_providers {
    hcloud = {
      source  = "hetznercloud/hcloud"
      version = "~> 1.45"
    }
    namecheap = {
      source  = "namecheap/namecheap"
      version = "~> 2.0"
    }
  }
}

# ---------------------------------------------------------------------------
# Variabili
# ---------------------------------------------------------------------------

variable "lb_name" {
  description = "Nome del Load Balancer Hetzner"
  type        = string
  default     = "aimulti-inbox-lb"
}

variable "lb_type" {
  description = "Tipo LB (lb11: fino a 3 servizi e 25 target — sufficiente per 80/443)"
  type        = string
  default     = "lb11"
}

variable "lb_location" {
  description = "Location Hetzner del LB (stessa dei nodi shared)"
  type        = string
  default     = "fsn1"
}

variable "target_ips" {
  description = "IP in rotazione HTTP, in ordine: il primo è il nodo 1 esistente (record A mail1, MX priorità 10); gli altri entrano in round_robin. I target type=ip Hetzner accettano IP cloud e Robot."
  type        = list(string)
  default     = ["2.28.99.252"]

  validation {
    condition     = length(var.target_ips) >= 1
    error_message = "Almeno un nodo deve restare in rotazione: il nodo 1 è l'MX mail1 primario."
  }
}

variable "landingo_port" {
  description = "Porta host di landing-go (EXPOSE 8080) usata dal health check del LB"
  type        = number
  default     = 8080
}

variable "zone" {
  description = "Zona DNS per i record MX SMTP (gestita su Namecheap)"
  type        = string
  default     = "aimulti.inbox"
}

variable "mx_ttl" {
  description = "TTL dei record MX/A mail (3600 in regime; 60-300 durante cutover — vedi LB.md §8)"
  type        = number
  default     = 3600
}

# ---------------------------------------------------------------------------
# Load Balancer — HTTP (landing/API/webmail) — design §3
# ---------------------------------------------------------------------------

resource "hcloud_load_balancer" "landing" {
  name               = var.lb_name
  load_balancer_type = var.lb_type
  location           = var.lb_location

  algorithm {
    type = "round_robin"
  }

  labels = {
    product   = "aimulti-inbox"
    component = "landing-http-lb"
  }
}

# HTTP 80 → nginx del nodo (static landing, /api/, proxy a landing-go e SOGo)
resource "hcloud_load_balancer_service" "http" {
  load_balancer_id = hcloud_load_balancer.landing.id
  protocol         = "http"
  listen_port      = 80
  destination_port = 80
  proxyprotocol    = false

  # Health check http su landing-go direttamente (bypassa nginx): il protocollo
  # del check è indipendente dal protocollo del servizio (API Hetzner).
  health_check {
    protocol = "http"
    port     = var.landingo_port
    interval = 10
    timeout  = 5
    retries  = 3

    http {
      path         = "/landingpage/api/status"
      status_codes = ["200"]
    }
  }
}

# HTTPS 443 → passthrough tcp: TLS resta sul nginx del nodo (SNI multi-brand,
# cert per-dominio già gestiti dal proxy nginx→landing-go/SOGo).
# Alternativa gestita al LB (protocol https + certificate managed Hetzner +
# cookie_name sticky per SOGo): vedi LB.md §7.
resource "hcloud_load_balancer_service" "https" {
  load_balancer_id = hcloud_load_balancer.landing.id
  protocol         = "tcp"
  listen_port      = 443
  destination_port = 443
  proxyprotocol    = false

  # Stesso check: app vivo su /landingpage/api/status anche per il servizio 443
  # (il check http è consentito su servizi tcp).
  health_check {
    protocol = "http"
    port     = var.landingo_port
    interval = 10
    timeout  = 5
    retries  = 3

    http {
      path         = "/landingpage/api/status"
      status_codes = ["200"]
    }
  }
}

# Target dinamici: lista IP (nodo 1 esistente + shared node creati da shared-node.tf).
# type "ip" = target non gestito dal LB, health check via percorso pubblico.
resource "hcloud_load_balancer_target" "nodes" {
  for_each         = toset(var.target_ips)
  load_balancer_id = hcloud_load_balancer.landing.id
  type             = "ip"
  ip               = each.value
}

# ---------------------------------------------------------------------------
# DNS MX multipli (SMTP) — design §3: il DNS fa il bilanciamento
# ---------------------------------------------------------------------------
#
# La zone è registrata su Namecheap (nameserver Namecheap): il provider
# namecheap scrive gli host records via API. Credenziali via env:
#   NAMECHEAP_USER_NAME / NAMECHEAP_API_USER / NAMECHEAP_API_KEY / NAMECHEAP_CLIENT_IP
#   (l'IP che chiama l'API va whitelisted nella console Namecheap)
# mode = MERGE: aggiorna SOLO i record dichiarati qui — gli altri record della
# zone (sito, www, SPF/DKIM) non vengono toccati. Se la zone fosse già gestita
# da Terraform altrove, fare prima `terraform import`.
#
# Priorità (mx_pref): 10 → mail1 (nodo 1 esistente), 20 → mail2 (nuovo shared
# node). Weight MX = 0 (RFC 5321): Namecheap non lo espone — il failover è per
# priorità, non per peso.
# NB: con node_count = 0 (nessun nodo 2) rimuovere anche il record MX 20,
# altrimenti resta un MX senza A record (i mittenti lo saltano, ma è sporco).

resource "namecheap_domain_records" "mx" {
  domain     = var.zone
  mode       = "MERGE"
  email_type = "MX"

  # mail1 → nodo 1 esistente (primo IP in target_ips)
  record {
    hostname = "mail1"
    type     = "A"
    address  = var.target_ips[0]
    ttl      = var.mx_ttl
  }

  # mail2 → primo shared node creato da shared-node.tf (richiede node_count >= 1)
  dynamic "record" {
    for_each = var.node_count >= 1 ? [1] : []
    content {
      hostname = "mail2"
      type     = "A"
      address  = [for s in hcloud_server.shared : s.ipv4_address][0]
      ttl      = var.mx_ttl
    }
  }

  # MX @: i mittenti esterni provano la priorità 10 (mail1) e ricadono sulla 20 (mail2)
  record {
    hostname = "@"
    type     = "MX"
    address  = "mail1.${var.zone}."
    mx_pref  = 10
    ttl      = var.mx_ttl
  }

  record {
    hostname = "@"
    type     = "MX"
    address  = "mail2.${var.zone}."
    mx_pref  = 20
    ttl      = var.mx_ttl
  }
}

# ---------------------------------------------------------------------------
# Output
# ---------------------------------------------------------------------------

output "lb_ipv4" {
  description = "IPv4 pubblico del Load Balancer (target dei record A/AAAA del sito in cutover HTTP)"
  value       = hcloud_load_balancer.landing.ipv4
}
