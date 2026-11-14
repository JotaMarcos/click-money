# ADR 0013 — Hospedagem em região brasileira

- Status: Proposta (aguarda confirmação do provedor e do orçamento)
- Data: 2026-11-13

## Contexto

A especificação exige hospedagem no Brasil, por latência e por conformidade com a LGPD, com PostgreSQL com PITR (RPO ≤ 5 min, RTO ≤ 1 h), ambientes isolados e infraestrutura como código. O provedor de nuvem ainda não foi escolhido.

## Decisão

**[PREMISSA]** AWS, região `sa-east-1` (São Paulo), descrita em OpenTofu:

- `api` e `worker` como contêineres em ECS Fargate, atrás de um Application Load Balancer.
- PostgreSQL gerenciado (RDS, Multi-AZ em produção) com backup contínuo para PITR.
- MongoDB a partir da Fase 2, em serviço gerenciado na mesma região.
- PWA em armazenamento de objetos com CDN, no mesmo domínio registrável da API (exigência do `SameSite=Strict`, ADR 0010).
- Segredos em gerenciador de segredos, com rotação.
- Três contas ou projetos isolados: dev, homologação e produção.

O código da aplicação não depende do provedor: tudo o que ele usa são contêineres, PostgreSQL, MongoDB, armazenamento de objetos e variáveis de ambiente.

## Alternativas

- **Google Cloud (`southamerica-east1`) ou Azure (Brazil South)**: equivalentes para este desenho. A troca afeta só `deploy/`.
- **Provedor nacional ou VPS**: menor custo, mas PITR, alta disponibilidade e rotação de segredos passam a ser trabalho do time.
- **Kubernetes**: descartado por ora. Custo operacional alto para dois binários.

## Consequências

- Enquanto o status for "Proposta", a Fase 0 entrega Dockerfile, `compose`, CI até o build e o scan da imagem, e os módulos de IaC sem aplicar. O deploy em homologação depende desta decisão.
- A latência entre São Paulo e o interior do Amazonas por satélite é dominada pelo enlace, não pela região; o desenho offline-first é o que resolve.
