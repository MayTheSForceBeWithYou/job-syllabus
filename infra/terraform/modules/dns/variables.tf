variable "subdomain" {
  description = "The delegated subdomain this project owns, e.g. job-syllabus.skopekreep.com. The apex domain (skopekreep.com) keeps its existing DNS/hosting (Squarespace) untouched - this zone only serves records under the subdomain, once the operator adds an NS delegation record for it at the registrar/DNS host."
  type        = string
}
