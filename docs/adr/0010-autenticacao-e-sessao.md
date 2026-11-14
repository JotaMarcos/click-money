# ADR 0010 — Autenticação e sessão

- Status: Aceita
- Data: 2026-11-13

## Contexto

O público tem pouca familiaridade digital e usa aparelhos compartilhados no balcão. O PWA precisa continuar utilizável offline por dias e voltar a sincronizar sem pedir senha a cada vez. Aparelhos são perdidos e roubados.

## Decisão

**Identificação.** O login é pelo telefone (E.164) e senha. E-mail é opcional.

**Senha.** Argon2id com memória de 19 MiB, 2 iterações, paralelismo 1, salt de 16 bytes e saída de 32 bytes (parâmetros mínimos recomendados pela OWASP). Os parâmetros ficam codificados junto do hash, o que permite elevá-los depois e refazer o hash no próximo login. Política: mínimo de 8 caracteres, sem regra de composição, com bloqueio de senhas muito comuns.

**Login por código (WhatsApp/SMS).** Fica atrás de feature flag e fora da Fase 1 **[PREMISSA]**, porque depende de provedor pago. A porta `Notifier` já existe para recebê-lo.

**Tokens.**

- Access token opaco, de 15 minutos, guardado no servidor como hash SHA-256.
- Refresh token opaco, de 60 dias **[PREMISSA]**, rotativo, guardado como hash e ligado a uma família e a um dispositivo.
- Reuso de um refresh token já trocado revoga a família inteira e registra evento de segurança.
- O período longo do refresh existe por causa do uso offline: o aparelho pode ficar semanas sem falar com o servidor.

**Navegador.** Os dois tokens vão em cookies `HttpOnly`, `Secure`, `SameSite=Strict`; o cookie de refresh tem `Path=/v1/auth`. Nada de token em `localStorage`. Toda requisição que altera estado exige o cabeçalho `X-CSRF-Token`, comparado com o valor de um cookie legível pelo script (double submit).

**Tenant ativo.** A sessão carrega o tenant ativo e o papel do usuário nele. O `tenant_id` nunca é aceito do corpo ou da URL.

**Autorização.** RBAC com os papéis Dono, Gerente e Caixa, verificado dentro do caso de uso, não no handler.

**Abuso.** Rate limit por IP e por telefone em login, cadastro e recuperação, com bloqueio progressivo. As respostas de login e de recuperação não revelam se o telefone existe.

## Alternativas

- **JWT assinado como access token**: descartado. Não permite revogação imediata por dispositivo sem uma lista de bloqueio, e a consulta ao banco por requisição é barata neste volume.
- **Sessão única de longa duração, sem refresh**: descartado. Um cookie roubado valeria por meses.
- **bcrypt ou scrypt**: descartados pela especificação e pela recomendação atual de Argon2id.

## Consequências

- Revogar um dispositivo tem efeito em no máximo 15 minutos online, e imediato no próximo push ou pull.
- Cada requisição autenticada faz uma leitura de sessão; um cache em memória de poucos segundos pode ser acrescentado se o teste de carga pedir.
- `SameSite=Strict` exige que API e PWA estejam no mesmo site (mesmo domínio registrável), o que condiciona o desenho de DNS.
- A chave que cifra os dados locais deriva de um segredo entregue no login; ao expirar o refresh, os dados locais ainda não sincronizados continuam cifrados e só são recuperados com novo login no mesmo aparelho.
