# terraform
AWS deployment for experiments: EC2 spot instances (t4g.small), one AZ,
Docker on each host, no RDS / ALB / NAT Gateway. Variables for shard and
coordinator counts. Always `terraform destroy` after a run.
Phase 4.
