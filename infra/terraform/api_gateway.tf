// API Gateway, Usage Plan, Cognito/Lambda Authorizer for Pincex

resource "aws_api_gateway_rest_api" "pincex" {
  name        = "pincex-api"
  description = "Pincex Unified API Gateway"
}

resource "aws_api_gateway_resource" "root" {
  rest_api_id = aws_api_gateway_rest_api.pincex.id
  parent_id   = aws_api_gateway_rest_api.pincex.root_resource_id
  path_part   = "v1"
}

resource "aws_api_gateway_method" "all" {
  rest_api_id   = aws_api_gateway_rest_api.pincex.id
  resource_id   = aws_api_gateway_resource.root.id
  http_method   = "ANY"
  authorization = "COGNITO_USER_POOLS" # or "CUSTOM" for Lambda authorizer
  authorizer_id = aws_api_gateway_authorizer.cognito.id # or .jwt.id for Lambda
  api_key_required = true
}

resource "aws_api_gateway_usage_plan" "main" {
  name = "pincex-usage-plan"
  api_stages {
    api_id = aws_api_gateway_rest_api.pincex.id
    stage  = aws_api_gateway_deployment.pincex.stage_name
  }
  throttle_settings {
    burst_limit = 100
    rate_limit  = 50
  }
  quota_settings {
    limit  = 10000
    period = "MONTH"
  }
}

resource "aws_api_gateway_api_key" "dev" {
  name      = "pincex-dev-key"
  enabled   = true
  stage_key {
    rest_api_id = aws_api_gateway_rest_api.pincex.id
    stage_name  = aws_api_gateway_deployment.pincex.stage_name
  }
}

resource "aws_api_gateway_usage_plan_key" "dev" {
  key_id        = aws_api_gateway_api_key.dev.id
  key_type      = "API_KEY"
  usage_plan_id = aws_api_gateway_usage_plan.main.id
}

resource "aws_cognito_user_pool" "pincex" {
  name = "pincex-user-pool"
}

resource "aws_api_gateway_authorizer" "cognito" {
  name                   = "CognitoAuthorizer"
  rest_api_id            = aws_api_gateway_rest_api.pincex.id
  identity_source        = "method.request.header.Authorization"
  type                   = "COGNITO_USER_POOLS"
  provider_arns          = [aws_cognito_user_pool.pincex.arn]
}

# Lambda authorizer example (uncomment if using custom JWT/OAuth2)
# resource "aws_lambda_function" "jwt_auth" {
#   ...
# }
# resource "aws_api_gateway_authorizer" "jwt" {
#   name                   = "JWTAuthorizer"
#   rest_api_id            = aws_api_gateway_rest_api.pincex.id
#   authorizer_uri         = "arn:aws:apigateway:${var.aws_region}:lambda:path/2015-03-31/functions/${aws_lambda_function.jwt_auth.arn}/invocations"
#   identity_source        = "method.request.header.Authorization"
#   type                   = "TOKEN"
# }

resource "aws_api_gateway_deployment" "pincex" {
  depends_on = [aws_api_gateway_method.all]
  rest_api_id = aws_api_gateway_rest_api.pincex.id
  stage_name  = "prod"
}

output "pincex_api_url" {
  value = aws_api_gateway_deployment.pincex.invoke_url
}
