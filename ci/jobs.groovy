// Job DSL seed — docs/design.md §10. Referenced from ci/jenkins.yaml's
// `jobs:` block, so these jobs exist from the very first boot, before
// anyone touches the UI.
//
// Only api-build, infra-plan, and infra-apply are defined here — the
// pipeline table in §10 also lists client-build (needs the Expo app,
// Phase 6), backfill and reextract (need the queue-driven ingestion from
// Phase 4). Add those jobs here when their phases land, not before.
//
// infra-plan is a simple push-triggered pipelineJob, not a full
// multibranch/PR-discovery job — §10 describes it as "PR touching
// infra/**", which really wants a multibranchPipelineJob with GitHub App
// or PAT credentials for PR discovery. Deferred: that needs a GitHub
// credential this seed doesn't have and shouldn't invent. Revisit once
// GitHub integration credentials exist.

def repoUrl = 'https://github.com/MayTheSForceBeWithYou/job-syllabus.git'

pipelineJob('api-build') {
    description('docs/design.md §10: go vet -> golangci-lint -> go test -race -cover -> extraction validation gate -> build -> Trivy -> ECR push ($GIT_SHA) -> ecs update-service -> poll stability -> smoke test /healthz')
    definition {
        cpsScm {
            scm {
                git {
                    remote { url(repoUrl) }
                    branch('*/main')
                }
            }
            scriptPath('ci/Jenkinsfile.api-build')
        }
    }
    triggers {
        // pollSCM, not githubPush(): GitHub's webhook servers can't reach
        // this Jenkins (ALB IP-locked to the operator only, §10) — see
        // ci/Jenkinsfile.api-build's comment for the full rationale. Set
        // here too, not just in the Jenkinsfile's own triggers{} block,
        // since this is what actually schedules the *first* build (a
        // declarative Jenkinsfile's triggers{} only takes over syncing
        // the job config starting from the second run).
        scm('H/5 * * * *')
    }
}

pipelineJob('infra-plan') {
    description('docs/design.md §10: fmt -check -> validate -> tfsec -> checkov -> plan, posted as a build artifact')
    definition {
        cpsScm {
            scm {
                git {
                    remote { url(repoUrl) }
                    branch('*/main')
                }
            }
            scriptPath('ci/Jenkinsfile.infra-plan')
        }
    }
    triggers {
        // pollSCM, not githubPush(): GitHub's webhook servers can't reach
        // this Jenkins (ALB IP-locked to the operator only, §10) — see
        // ci/Jenkinsfile.api-build's comment for the full rationale. Set
        // here too, not just in the Jenkinsfile's own triggers{} block,
        // since this is what actually schedules the *first* build (a
        // declarative Jenkinsfile's triggers{} only takes over syncing
        // the job config starting from the second run).
        scm('H/5 * * * *')
    }
}

pipelineJob('infra-apply') {
    description('docs/design.md §10: manual only, after infra-plan — input approval -> apply. No automatic trigger, deliberately.')
    definition {
        cpsScm {
            scm {
                git {
                    remote { url(repoUrl) }
                    branch('*/main')
                }
            }
            scriptPath('ci/Jenkinsfile.infra-apply')
        }
    }
}
