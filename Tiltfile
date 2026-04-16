# Tiltfile — default dispatcher
#
# Two modes are available:
#   • Remote (DockerHub images):  tilt up                (or: tilt up -f Tiltfile-remote)
#   • Local  (build from source): tilt up -f Tiltfile-local
#
# This file loads Tiltfile-remote by default so `tilt up` without a -f flag
# keeps working as before.

print("ℹ️  Default mode: REMOTE (DockerHub). For local source builds: tilt up -f Tiltfile-local")

include('./Tiltfile-remote')
