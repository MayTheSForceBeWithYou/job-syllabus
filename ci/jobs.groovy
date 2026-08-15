// Job DSL seed — docs/design.md §10. Referenced from ci/jenkins.yaml's
// `jobs:` block, so these jobs exist from the very first boot, before
// anyone touches the UI.
//
// api-build, worker-build, ingest-build, rollup-build, infra-plan,
// infra-apply, backfill, and reextract are defined here as of Phase 5 — the
// pipeline table in §10 also lists client-build (needs the Expo app, Phase
// 6). Add that job here when its phase lands, not before.
//
// ingest-build/rollup-build exist because modules/task-scheduled's
// scheduled RunTask targets need a real, versioned image to pull — a real
// run confirmed the gap directly: the first-ever `run-task` invocation of
// job-syllabus-ingest failed outright with CannotPullContainerError,
// since Terraform's bootstrap task definition only ever pointed at a
// placeholder `:latest` tag nothing had pushed. Same build/Trivy/register
// pattern as api-build/worker-build, minus any `ecs update-service` step
// — these back one-shot scheduled tasks, not long-running services, so
// registering a new task definition revision against the family (not a
// pinned revision, see modules/task-scheduled's ecs_parameters) is the
// entire "deploy."
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

pipelineJob('worker-build') {
    description('docs/design.md §9/§10: go vet -> golangci-lint -> go test -race -cover -> extraction validation gate -> build -> Trivy -> ECR push ($GIT_SHA) -> ecs update-service -> smoke test (force a task up, confirm its startup log line)')
    definition {
        cpsScm {
            scm {
                git {
                    remote { url(repoUrl) }
                    branch('*/main')
                }
            }
            scriptPath('ci/Jenkinsfile.worker-build')
        }
    }
    triggers {
        // Same rationale as api-build's trigger — see that job's comment.
        scm('H/5 * * * *')
    }
}

pipelineJob('backfill') {
    description('docs/design.md §5: on-demand re-ingest of a single company (COMPANY_SLUG param) without waiting for the daily 06:00 UTC schedule. Manual only.')
    definition {
        cpsScm {
            scm {
                git {
                    remote { url(repoUrl) }
                    branch('*/main')
                }
            }
            scriptPath('ci/Jenkinsfile.backfill')
        }
    }
}

pipelineJob('ingest-build') {
    description('docs/design.md §5/§9: go vet -> golangci-lint -> go test -race -cover -> extraction validation gate -> build -> Trivy -> ECR push ($GIT_SHA) -> register task definition revision -> smoke test (run-task, single company)')
    definition {
        cpsScm {
            scm {
                git {
                    remote { url(repoUrl) }
                    branch('*/main')
                }
            }
            scriptPath('ci/Jenkinsfile.ingest-build')
        }
    }
    triggers {
        scm('H/5 * * * *')
    }
}

pipelineJob('rollup-build') {
    description('docs/design.md §4/§5/§9: go vet -> golangci-lint -> go test -race -cover -> build -> Trivy -> ECR push ($GIT_SHA) -> register task definition revision -> smoke test (run reconcile)')
    definition {
        cpsScm {
            scm {
                git {
                    remote { url(repoUrl) }
                    branch('*/main')
                }
            }
            scriptPath('ci/Jenkinsfile.rollup-build')
        }
    }
    triggers {
        scm('H/5 * * * *')
    }
}

pipelineJob('reextract') {
    description('docs/design.md §6 "Re-extraction": re-enqueues every posting whose ExtractVer is behind the current dictionary/prompt version (cmd/rollup reextract) via job-syllabus-rollup-reconcile\'s task definition, command overridden. Manual only.')
    definition {
        cpsScm {
            scm {
                git {
                    remote { url(repoUrl) }
                    branch('*/main')
                }
            }
            scriptPath('ci/Jenkinsfile.reextract')
        }
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
