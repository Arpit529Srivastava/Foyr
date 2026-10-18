# Public front door for customer apps: HTTPS in, authproxy behind it (ADR-031).
resource "aws_security_group" "alb" {
  name        = "foyr-alb"
  description = "Public load balancer for apps"
  vpc_id      = aws_vpc.main.id
  tags        = { Name = "foyr-alb" }
}

resource "aws_vpc_security_group_ingress_rule" "alb_https" {
  security_group_id = aws_security_group.alb.id
  description       = "HTTPS from anywhere"
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
  cidr_ipv4         = "0.0.0.0/0"
}

resource "aws_vpc_security_group_ingress_rule" "alb_http" {
  security_group_id = aws_security_group.alb.id
  description       = "HTTP from anywhere, only to redirect to HTTPS"
  ip_protocol       = "tcp"
  from_port         = 80
  to_port           = 80
  cidr_ipv4         = "0.0.0.0/0"
}

resource "aws_vpc_security_group_egress_rule" "alb_to_authproxy" {
  security_group_id            = aws_security_group.alb.id
  description                  = "Only to authproxy"
  ip_protocol                  = "tcp"
  from_port                    = local.authproxy_port
  to_port                      = local.authproxy_port
  referenced_security_group_id = aws_security_group.authproxy.id
}

resource "aws_lb" "apps" {
  name                       = "foyr-apps"
  load_balancer_type         = "application"
  internal                   = false
  subnets                    = aws_subnet.public[*].id
  security_groups            = [aws_security_group.alb.id]
  drop_invalid_header_fields = true
  idle_timeout               = 120
}

resource "aws_lb_target_group" "authproxy" {
  name                 = "foyr-authproxy"
  target_type          = "ip"
  protocol             = "HTTP"
  port                 = local.authproxy_port
  vpc_id               = aws_vpc.main.id
  deregistration_delay = 30

  health_check {
    path                = "/_foyr/healthz"
    matcher             = "200"
    interval            = 15
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }
}

resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.apps.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = aws_acm_certificate_validation.apps.certificate_arn

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.authproxy.arn
  }
}

resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.apps.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type = "redirect"
    redirect {
      protocol    = "HTTPS"
      port        = "443"
      status_code = "HTTP_301"
    }
  }
}
