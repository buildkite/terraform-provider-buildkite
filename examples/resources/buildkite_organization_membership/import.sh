# Import an active membership using the user's UUID, not a GraphQL membership ID.
terraform import buildkite_organization_membership.jane 0185dbbf-8447-4f72-ac7e-4ea3c2ec8381

# Import a pending invitation using its UUID with the invitation/ prefix.
terraform import buildkite_organization_membership.new_user invitation/00000000-0000-4000-8000-000000000001
