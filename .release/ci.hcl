# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

schema = "1"

project "nomad-enterprise" {
  team = "nomad"

  slack {
    notification_channel = "CUYKT2A73"
  }

  github {
    organization = "hashicorp"
    repository   = "nomad-enterprise"

    release_branches = [
      "main",
      "release/**",
    ]
  }
}

event "build" {
  action "build" {
    organization = "hashicorp"
    repository   = "nomad-enterprise"
    workflow     = "build"
  }
}

event "prepare" {
  depends = ["build"]

  action "prepare" {
    organization = "hashicorp"
    repository   = "crt-workflows-common"
    workflow     = "prepare"
    depends      = ["build"]
  }

  notification {
    on = "always"
  }
}

event "quality-tests" {
  depends = ["upload-dev"]
  action "quality-tests" {
    organization = "hashicorp"
    repository   = "crt-workflows-common"
    workflow     = "quality-tests"
  }

  notification {
    on = "fail"
  }
}

## These are promotion and post-publish events
## they should be added to the end of the file after the prepare event stanza.

event "trigger-staging" {
  // This event is dispatched by the bob trigger-promotion command  // and is required - do not delete.
}

event "promote-staging" {
  depends = ["trigger-staging"]

  action "promote-staging" {
    organization = "hashicorp"
    repository   = "crt-workflows-common"
    workflow     = "promote-staging"
    config       = "release-metadata.hcl"
  }

  notification {
    on = "always"
  }
}

# TODO(tgross): docker image release commented-out for 1.6.0-beta.1 so that we
# can ship the beta while debugging the release pipeline. The image should ship
# with 1.6.0-rc1 or the GA
#
# event "promote-staging-docker" {
#   depends = ["promote-staging"]

#   action "promote-staging-docker" {
#     organization = "hashicorp"
#     repository   = "crt-workflows-common"
#     workflow     = "promote-staging-docker"
#   }

#   notification {
#     on = "always"
#   }
# }

event "trigger-production" {
  // This event is dispatched by the bob trigger-promotion command  // and is required - do not delete.
}

event "promote-production" {
  depends = ["trigger-production"]

  action "promote-production" {
    organization = "hashicorp"
    repository   = "crt-workflows-common"
    workflow     = "promote-production"
  }

  notification {
    on = "always"
  }
}

# TODO(tgross): docker image release commented-out for 1.6.0-beta.1 so that we
# can ship the beta while debugging the release pipeline. The image should ship
# with 1.6.0-rc1 or the GA
#
# event "promote-production-docker" {
#   depends = ["promote-production"]

#   action "promote-production-docker" {
#     organization = "hashicorp"
#     repository   = "crt-workflows-common"
#     workflow     = "promote-production-docker"
#   }

#   notification {
#     on = "always"
#   }
# }

event "promote-production-packaging" {

  # TODO(tgross): see above
  # depends = ["promote-production-docker"]

  depends = ["promote-production"]

  action "promote-production-packaging" {
    organization = "hashicorp"
    repository   = "crt-workflows-common"
    workflow     = "promote-production-packaging"
  }

  notification {
    on = "always"
  }
}

