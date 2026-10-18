variable "region" {
  description = "AWS region for all resources."
  type        = string
  default     = "ap-south-1"
}

variable "vpc_cidr" {
  description = "Private address range for the whole VPC."
  type        = string
  default     = "10.0.0.0/16"
}

variable "az_count" {
  description = "How many availability zones to spread across."
  type        = number
  default     = 2
}

variable "apps_domain" {
  description = "APPS_DOMAIN for customer apps; temporary until the separate domain is bought (ADR-028, docs/domain-switch.md)."
  type        = string
  default     = "apps.foyr.online"
}

variable "authproxy_image_tag" {
  description = "Tag in foyr/authproxy for the first task definition; the image pipeline deploys later tags."
  type        = string
  default     = "bootstrap"
}

variable "authproxy_static_routes" {
  description = "slug=host:port routes until foyrd runs in AWS (step 8c, ADR-031); empty switches to foyrd's routes."
  type        = string
  default     = "foyr-hello=d-4.apps.foyr.internal:3000"
}
