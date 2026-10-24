# auth.<platform_domain> (sign-in pages) and app.<platform_domain> (dashboard) are served by foyrd behind the
# same load balancer as api. (ADR-035). foyr.online itself stays at GoDaddy, so the validation records and the
# two CNAMEs are added there by hand; the outputs list them.
locals {
  auth_domain    = "auth.${var.platform_domain}"
  app_domain     = "app.${var.platform_domain}"
  platform_hosts = [local.auth_domain, local.app_domain]
}

resource "aws_acm_certificate" "platform_hosts" {
  domain_name               = local.auth_domain
  subject_alternative_names = [local.app_domain]
  validation_method         = "DNS"

  lifecycle {
    create_before_destroy = true
  }
}

# Waits for issuance, which needs the validation records at GoDaddy first. Apply once with
# platform_hosts_live = false, add the records, then apply again with it true.
resource "aws_acm_certificate_validation" "platform_hosts" {
  count           = var.platform_hosts_live ? 1 : 0
  certificate_arn = aws_acm_certificate.platform_hosts.arn
}

# The load balancer picks the certificate by hostname (SNI).
resource "aws_lb_listener_certificate" "platform_hosts" {
  count           = var.platform_hosts_live ? 1 : 0
  listener_arn    = aws_lb_listener.https.arn
  certificate_arn = aws_acm_certificate_validation.platform_hosts[0].certificate_arn
}
