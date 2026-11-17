provider "aws" {
  region = "us-east-1"
}

resource "aws_vpc" "main" {
  cidr_block           = "10.0.0.0/16"
  enable_dns_hostnames = true

  tags = {
    Name = "pipescope-prod-vpc"
  }
}

resource "aws_ecr_repository" "api" {
  name                 = "pipescope/api"
  image_tag_mutability = "MUTABLE"
}

resource "aws_eks_cluster" "main" {
  name     = "prod-cluster"
  role_arn = "arn:aws:iam::123456789012:role/eks-cluster-role"

  vpc_config {
    vpc_id     = aws_vpc.main.id
    subnet_ids = ["subnet-11111111", "subnet-22222222"]
  }
}

resource "aws_instance" "app_servers" {
  ami           = "ami-0c55b159cbfafe1f0"
  instance_type = "t3.medium"
  vpc_id        = aws_vpc.main.id

  tags = {
    Name = "app_servers"
    Role = "app_servers"
  }
}
