# Import an organization member using its GraphQL ID
#
# You can use the following GraphQL query to find associated organization members across your organization - which
# you may require to paginate through by feeding in the `endCursor` fields within the `members` query arguments (see
# the GraphQL documentation for more information at https://buildkite.com/docs/apis/graphql/schemas/object/organization):
# query getOrganizationMembers {
#   organization(slug: "ORGANIZATION"){
#     members(first: 50, order:NAME, after:""){
# .     pageInfo{
#         hasNextPage
#         endCursor
# .     }
#       edges{ 
#         node{
#           id
#           user{
#             name
#             email
#           }
#         }
#       }
#     }
#   }
# }
#
# For organization administrators, you are also able to obtain an organization member's UUID through each User's settings 
# page within a specific organization's settings. Visit https://buildkite.com/organizations/~/users for your currently
# selected organization's settings and select a specific user to see their UUID within the URL.
#
# Upon obtaining the UUID (used as MEMBER_UUID below), you can use the following GraphQL query to fetch its GraphQL ID:
# query getOrganizationMemberBySlug {
#   oorganizationMember(slug: "ORGANIZATION/MEMBER_UUUD"){
#     id
#     user {
#       name
# .     email
#     }
#   }
# }

terraform import buildkite_organization_member.john_doe T3JnYW5pemF0aW9uTWVtYmVyLS0tYTlhZWU1ZGYtNDFjZS00YTY1LThkNjctM2E3OWNlM2Q1Y2Y5
