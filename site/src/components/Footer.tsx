import { Container } from '@/components/Container'

export function Footer() {
  return (
    <footer style={{ backgroundColor: '#0a0a0f' }}>
      <Container>
        <div className="border-t border-zinc-800 py-6">
          <p className="text-sm text-zinc-500">
            Copyright &copy; {new Date().getFullYear()} Branchd Inc. All rights
            reserved.
          </p>
        </div>
      </Container>
    </footer>
  )
}
