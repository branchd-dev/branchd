'use client'

import { Container } from '@/components/Container'

const steps = [
  {
    step: 1,
    title: 'Deploy with CloudFormation',
    description:
      'One-click deployment to AWS with pre-configured infrastructure',
    details: [
      {
        label: 'Launch Stack',
        content: (
          <div className="space-y-3">
            <a
              href="https://console.aws.amazon.com/cloudformation/home#/stacks/create/review?templateURL=https://branchd-cloudformation-templates.s3.amazonaws.com/branchd.yaml&stackName=branchd"
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-2 rounded-lg bg-gradient-to-r from-violet-600 to-violet-500 px-4 py-2 text-sm font-medium text-white transition-all hover:from-violet-500 hover:to-violet-400"
            >
              <svg className="h-5 w-5" fill="currentColor" viewBox="0 0 20 20">
                <path d="M10.894 2.553a1 1 0 00-1.788 0l-7 14a1 1 0 001.169 1.409l5-1.429A1 1 0 009 15.571V11a1 1 0 112 0v4.571a1 1 0 00.725.962l5 1.428a1 1 0 001.17-1.408l-7-14z" />
              </svg>
              Launch Stack on AWS
            </a>
            <p className="text-sm text-zinc-400">
              CloudFormation will create the instance, security groups, and
              storage automatically. Setup takes ~5 minutes.
            </p>
          </div>
        ),
      },
      {
        label: 'Sizing tips',
        content: (
          <div className="space-y-3 text-sm text-zinc-300">
            <ul className="space-y-2 text-zinc-400">
              <li>
                • <strong className="text-zinc-300">Instance size:</strong>{' '}
                start with 1/2 or 1/4 of your production instance size.
              </li>
              <li>
                • <strong className="text-zinc-300">Storage size:</strong> start
                with 1 to 1.2x your production database size. Branchd uses
                compression and copy-on-write for efficient storage.
              </li>
            </ul>
            <div>
              <p className="mb-2 text-zinc-400">Check your database size:</p>
              <pre className="overflow-x-auto rounded-lg bg-zinc-900 p-3 text-sm text-zinc-300">
                <code>{`psql "postgresql://user:pass@host:port/dbname" -c "SELECT pg_size_pretty(pg_database_size(current_database())) as size;"`}</code>
              </pre>
            </div>
          </div>
        ),
      },
      {
        label: 'Instance types reference',
        content: (
          <div className="space-y-3">
            <div>
              <p className="mb-2 text-sm font-medium text-violet-400">Basic</p>
              <div className="space-y-2 text-sm">
                <div className="grid grid-cols-[1fr_auto_auto] gap-3 rounded bg-zinc-800/50 px-3 py-2">
                  <span className="text-zinc-400">t4g.micro</span>
                  <span className="text-right text-zinc-500 tabular-nums">
                    1 vCPU
                  </span>
                  <span className="w-12 text-right text-zinc-500 tabular-nums">
                    1GB
                  </span>
                </div>
                <div className="grid grid-cols-[1fr_auto_auto] gap-3 rounded bg-zinc-800/50 px-3 py-2">
                  <span className="text-zinc-400">t4g.small</span>
                  <span className="text-right text-zinc-500 tabular-nums">
                    1 vCPU
                  </span>
                  <span className="w-12 text-right text-zinc-500 tabular-nums">
                    2GB
                  </span>
                </div>
              </div>
            </div>
            <div>
              <p className="mb-2 text-sm font-medium text-violet-400">
                Standard
              </p>
              <div className="space-y-2 text-sm">
                <div className="grid grid-cols-[1fr_auto_auto] gap-3 rounded bg-zinc-800/50 px-3 py-2">
                  <span className="text-zinc-400">m6g.medium</span>
                  <span className="text-right text-zinc-500 tabular-nums">
                    1 vCPU
                  </span>
                  <span className="w-12 text-right text-zinc-500 tabular-nums">
                    4GB
                  </span>
                </div>
                <div className="grid grid-cols-[1fr_auto_auto] gap-3 rounded bg-zinc-800/50 px-3 py-2">
                  <span className="text-zinc-400">t4g.large</span>
                  <span className="text-right text-zinc-500 tabular-nums">
                    2 vCPU
                  </span>
                  <span className="w-12 text-right text-zinc-500 tabular-nums">
                    8GB
                  </span>
                </div>
                <div className="grid grid-cols-[1fr_auto_auto] gap-3 rounded bg-zinc-800/50 px-3 py-2">
                  <span className="text-zinc-400">t4g.xlarge</span>
                  <span className="text-right text-zinc-500 tabular-nums">
                    4 vCPU
                  </span>
                  <span className="w-12 text-right text-zinc-500 tabular-nums">
                    16GB
                  </span>
                </div>
                <div className="grid grid-cols-[1fr_auto_auto] gap-3 rounded bg-zinc-800/50 px-3 py-2">
                  <span className="text-zinc-400">t4g.2xlarge</span>
                  <span className="text-right text-zinc-500 tabular-nums">
                    8 vCPU
                  </span>
                  <span className="w-12 text-right text-zinc-500 tabular-nums">
                    32GB
                  </span>
                </div>
                <div className="grid grid-cols-[1fr_auto_auto] gap-3 rounded bg-zinc-800/50 px-3 py-2">
                  <span className="text-zinc-400">m6g.4xlarge</span>
                  <span className="text-right text-zinc-500 tabular-nums">
                    16 vCPU
                  </span>
                  <span className="w-12 text-right text-zinc-500 tabular-nums">
                    64GB
                  </span>
                </div>
                <div className="grid grid-cols-[1fr_auto_auto] gap-3 rounded bg-zinc-800/50 px-3 py-2">
                  <span className="text-zinc-400">m6g.8xlarge</span>
                  <span className="text-right text-zinc-500 tabular-nums">
                    32 vCPU
                  </span>
                  <span className="w-12 text-right text-zinc-500 tabular-nums">
                    128GB
                  </span>
                </div>
                <div className="grid grid-cols-[1fr_auto_auto] gap-3 rounded bg-zinc-800/50 px-3 py-2">
                  <span className="text-zinc-400">m6g.12xlarge</span>
                  <span className="text-right text-zinc-500 tabular-nums">
                    48 vCPU
                  </span>
                  <span className="w-12 text-right text-zinc-500 tabular-nums">
                    192GB
                  </span>
                </div>
                <div className="grid grid-cols-[1fr_auto_auto] gap-3 rounded bg-zinc-800/50 px-3 py-2">
                  <span className="text-zinc-400">m6g.16xlarge</span>
                  <span className="text-right text-zinc-500 tabular-nums">
                    64 vCPU
                  </span>
                  <span className="w-12 text-right text-zinc-500 tabular-nums">
                    256GB
                  </span>
                </div>
              </div>
            </div>
            <div>
              <p className="mb-2 text-sm font-medium text-violet-400">
                Memory-Optimized
              </p>
              <div className="space-y-2 text-sm">
                <div className="grid grid-cols-[1fr_auto_auto] gap-3 rounded bg-zinc-800/50 px-3 py-2">
                  <span className="text-zinc-400">r6g.large</span>
                  <span className="text-right text-zinc-500 tabular-nums">
                    2 vCPU
                  </span>
                  <span className="w-12 text-right text-zinc-500 tabular-nums">
                    16GB
                  </span>
                </div>
                <div className="grid grid-cols-[1fr_auto_auto] gap-3 rounded bg-zinc-800/50 px-3 py-2">
                  <span className="text-zinc-400">r6g.xlarge</span>
                  <span className="text-right text-zinc-500 tabular-nums">
                    4 vCPU
                  </span>
                  <span className="w-12 text-right text-zinc-500 tabular-nums">
                    32GB
                  </span>
                </div>
                <div className="grid grid-cols-[1fr_auto_auto] gap-3 rounded bg-zinc-800/50 px-3 py-2">
                  <span className="text-zinc-400">r6g.2xlarge</span>
                  <span className="text-right text-zinc-500 tabular-nums">
                    8 vCPU
                  </span>
                  <span className="w-12 text-right text-zinc-500 tabular-nums">
                    64GB
                  </span>
                </div>
                <div className="grid grid-cols-[1fr_auto_auto] gap-3 rounded bg-zinc-800/50 px-3 py-2">
                  <span className="text-zinc-400">r6g.4xlarge</span>
                  <span className="text-right text-zinc-500 tabular-nums">
                    16 vCPU
                  </span>
                  <span className="w-12 text-right text-zinc-500 tabular-nums">
                    128GB
                  </span>
                </div>
                <div className="grid grid-cols-[1fr_auto_auto] gap-3 rounded bg-zinc-800/50 px-3 py-2">
                  <span className="text-zinc-400">r6g.8xlarge</span>
                  <span className="text-right text-zinc-500 tabular-nums">
                    32 vCPU
                  </span>
                  <span className="w-12 text-right text-zinc-500 tabular-nums">
                    256GB
                  </span>
                </div>
              </div>
            </div>
          </div>
        ),
      },
    ],
  },
  {
    step: 2,
    title: 'Install CLI',
    description: 'Download the Branchd CLI on your local machine',
    details: [
      {
        label: 'Installation',
        content: (
          <div className="space-y-3">
            <pre className="overflow-x-auto rounded-lg bg-zinc-900 p-4 text-sm text-zinc-300">
              <code>{`# Linux/macOS
curl -fsSL https://raw.githubusercontent.com/branchd-dev/branchd/main/install-cli.sh | bash`}</code>
            </pre>
            <p className="text-sm text-zinc-400">
              Or download from{' '}
              <a
                href="https://github.com/branchd-dev/branchd/releases"
                target="_blank"
                rel="noopener noreferrer"
                className="text-violet-400 hover:text-violet-300"
              >
                GitHub Releases
              </a>
            </p>
          </div>
        ),
      },
    ],
  },
  {
    step: 3,
    title: 'Configure Your Instance',
    description: 'Complete the web-based setup wizard',
    details: [
      {
        label: 'Setup Wizard',
        content: (
          <div className="space-y-3">
            <pre className="overflow-x-auto rounded-lg bg-zinc-900 p-4 text-sm text-zinc-300">
              <code>{`# Initialize CLI with your instance IP
branchd init <your-instance-ip>`}</code>
            </pre>
            <p className="text-sm text-zinc-400">
              This creates branchd.json and opens the setup page in your
              browser.
            </p>
            <ul className="space-y-1 text-sm text-zinc-400">
              <li>• Create your admin account</li>
              <li>• Configure source database connection</li>
              <li>
                • Optionally set up anonymization rules and refresh schedule
              </li>
              <li>• Trigger initial database restore</li>
            </ul>
          </div>
        ),
      },
    ],
  },
  {
    step: 4,
    title: 'Create Your First Branch',
    description:
      'After the initial restore completes, use the CLI to create and manage branches',
    details: [
      {
        label: 'Branch Management',
        content: (
          <div className="space-y-3">
            <pre className="overflow-x-auto rounded-lg bg-zinc-900 p-4 text-sm text-zinc-300">
              <code>{`# Login to your instance
branchd login --email=admin@example.com

# Create a new branch - returns ready-to-use connection string
branchd checkout my-feature-branch

# List all branches
branchd ls

# Delete a branch
branchd delete my-feature-branch`}</code>
            </pre>
          </div>
        ),
      },
    ],
  },
]

export function PrimaryFeatures() {
  return (
    <section
      id="get-started"
      aria-label="Getting started with Branchd"
      className="relative overflow-hidden pt-20 pb-28 sm:py-32"
      style={{ backgroundColor: '#0a0a0f' }}
    >
      <Container className="relative">
        <div className="max-w-3xl md:mx-auto md:text-center">
          <h2 className="font-display text-2xl tracking-tight text-white sm:text-3xl md:text-4xl">
            Get Started in Minutes
          </h2>
          <p className="mt-4 text-base tracking-tight text-zinc-400">
            From zero to branching in four simple steps. No complex
            configuration.
          </p>
        </div>

        <div className="mt-16 md:mt-20">
          {/* Desktop: Timeline view */}
          <div className="hidden lg:block">
            <div className="space-y-12">
              {steps.map((step, stepIndex) => (
                <div key={step.step} className="relative">
                  {/* Connector line */}
                  {stepIndex < steps.length - 1 && (
                    <div className="absolute top-16 bottom-0 left-6 w-px bg-gradient-to-b from-violet-500/50 to-transparent" />
                  )}

                  <div className="flex gap-8">
                    {/* Step number */}
                    <div className="flex-shrink-0">
                      <div className="flex h-12 w-12 items-center justify-center rounded-full bg-violet-600/20 ring-1 ring-violet-500/30">
                        <span className="text-lg font-semibold text-violet-400">
                          {step.step}
                        </span>
                      </div>
                    </div>

                    {/* Content */}
                    <div className="flex-1 pb-12">
                      <h3 className="font-display text-xl text-white">
                        {step.title}
                      </h3>
                      <p className="mt-2 text-sm text-zinc-400">
                        {step.description}
                      </p>

                      <div className="mt-6 space-y-6">
                        {step.details.map((detail) => (
                          <div key={detail.label}>
                            <h4 className="mb-3 text-sm font-medium text-zinc-300">
                              {detail.label}
                            </h4>
                            <div className="rounded-lg bg-zinc-900/50 p-4 ring-1 ring-zinc-800">
                              {detail.content}
                            </div>
                          </div>
                        ))}
                      </div>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Mobile: Stacked cards */}
          <div className="space-y-8 lg:hidden">
            {steps.map((step) => (
              <div
                key={step.step}
                className="rounded-2xl bg-zinc-900/50 p-6 ring-1 ring-zinc-800"
              >
                <div className="flex items-start gap-4">
                  <div className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-full bg-violet-600/20 ring-1 ring-violet-500/30">
                    <span className="text-base font-semibold text-violet-400">
                      {step.step}
                    </span>
                  </div>
                  <div className="flex-1">
                    <h3 className="font-display text-lg text-white">
                      {step.title}
                    </h3>
                    <p className="mt-2 text-sm text-zinc-400">
                      {step.description}
                    </p>
                  </div>
                </div>

                <div className="mt-6 space-y-6">
                  {step.details.map((detail) => (
                    <div key={detail.label}>
                      <h4 className="mb-3 text-sm font-medium text-zinc-300">
                        {detail.label}
                      </h4>
                      <div className="rounded-lg bg-zinc-900 p-4 ring-1 ring-zinc-800">
                        {detail.content}
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Bottom CTA */}
        <div className="mt-16 text-center">
          <p className="text-base text-zinc-400">
            Need support?{' '}
            <a
              href="mailto:rafael@branchd.dev?subject=Branchd%20Support"
              target="_blank"
              rel="noopener noreferrer"
              className="text-violet-400 hover:text-violet-300"
            >
              Reach out
            </a>
            .
          </p>
        </div>
      </Container>
    </section>
  )
}
