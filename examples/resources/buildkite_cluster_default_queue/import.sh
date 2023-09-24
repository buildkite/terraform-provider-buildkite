# import a clusters default queue resource using the GraphQL ID of the cluster itself
#
# you can use this query to find the ID:
# query getClusters {
#   organization(slug: "ORGANIZATION"){
#     clusters(first: 5, order:NAME) {
#       edges{
#         node {
#           id
#           name
#         }
#       }
#     }
#   }
# }
terraform import buildkite_cluster_default_queue.primary Q2x1c3Rlci0tLTI3ZmFmZjA4LTA3OWEtNDk5ZC1hMmIwLTIzNmY3NWFkMWZjYg==
