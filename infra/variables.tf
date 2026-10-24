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

variable "api_domain" {
  description = "foyrd's public hostname (PLATFORM_DOMAIN api.), for GitHub webhooks."
  type        = string
  default     = "api.foyr.online"
}

variable "foyrd_image_tag" {
  description = "Tag in foyr/foyrd for the first task definition; the image pipeline deploys later tags."
  type        = string
  default     = "bootstrap"
}

variable "github_app_id" {
  description = "GitHub App foyrd acts as (foyr-dev for now, ADR-023); not secret."
  type        = string
  default     = "5261051"
}

variable "platform_domain" {
  description = "PLATFORM_DOMAIN: the site, app., api. and auth. hosts (ADR-001, ADR-015)."
  type        = string
  default     = "foyr.online"
}

variable "platform_hosts_live" {
  description = "Set true once the DNS validation records for auth. and app. exist at GoDaddy (see the platform_hosts_validation_records output): only then can the certificate be attached."
  type        = bool
  default     = false
}
