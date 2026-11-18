# Unarchive through the API first if archived, then import the REST UUID (not the GraphQL ID).
# The Target must be active before an Instance can be unarchived.
terraform import buildkite_component_instance.api 01a1b2c3-4d5e-7f60-8a1b-2c3d4e5f6a7b
