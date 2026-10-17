# apps.foyr.online is delegated here from GoDaddy with 4 NS records (ADR-031).
resource "aws_route53_zone" "apps" {
  name    = var.apps_domain
  comment = "Customer apps, <slug>.${var.apps_domain}"
}

# One certificate for every app: *.apps.foyr.online.
resource "aws_acm_certificate" "apps" {
  domain_name       = "*.${var.apps_domain}"
  validation_method = "DNS"

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_route53_record" "apps_cert" {
  for_each = {
    for o in aws_acm_certificate.apps.domain_validation_options : o.domain_name => o
  }
  zone_id         = aws_route53_zone.apps.zone_id
  name            = each.value.resource_record_name
  type            = each.value.resource_record_type
  records         = [each.value.resource_record_value]
  ttl             = 300
  allow_overwrite = true
}

# Waits until AWS sees the record, which needs the GoDaddy NS records first.
resource "aws_acm_certificate_validation" "apps" {
  certificate_arn         = aws_acm_certificate.apps.arn
  validation_record_fqdns = [for r in aws_route53_record.apps_cert : r.fqdn]
}

# Every <slug>.apps.foyr.online goes to the load balancer.
resource "aws_route53_record" "apps_wildcard" {
  zone_id = aws_route53_zone.apps.zone_id
  name    = "*.${var.apps_domain}"
  type    = "A"

  alias {
    name                   = aws_lb.apps.dns_name
    zone_id                = aws_lb.apps.zone_id
    evaluate_target_health = false
  }
}
