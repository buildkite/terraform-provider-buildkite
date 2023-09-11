# import a cluster queue resource using the GraphQL ID along with its respective cluster UUID
#
# you can use this query to find the ID:
# query getClusterQueues {
#   organization(slug: "ORGANIZATION_SLUG") {
#     cluster(id: "CLUSTER_UUID") {
#       queues(first: 50) {
#         edges {
#           node {
#             id
#             key
#           }
#         }
#       }
#     }
#   }
# }
terraform import buildkite_cluster_queue.test Q2x1c3RlclF1ZXVlLS0tYjJiOGRhNTEtOWY5My00Y2MyLTkyMjktMGRiNzg3ZDQzOTAz,35498aaf-ad05-4fa5-9a07-91bf6cacd2bd
