output "table_name" {
  value = module.data.table_name
}

output "table_arn" {
  value = module.data.table_arn
}

output "data_bucket" {
  value = module.data.data_bucket
}

output "data_bucket_arn" {
  value = module.data.data_bucket_arn
}

output "ecr_repository_urls" {
  value = module.data.ecr_repository_urls
}

output "dns_zone_id" {
  value = module.dns.zone_id
}

output "dns_name_servers" {
  value = module.dns.name_servers
}

output "dns_subdomain" {
  value = module.dns.subdomain
}
