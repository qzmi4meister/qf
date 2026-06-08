export default function Mark({ size = 24 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" aria-hidden>
      {/* packet traffic lines */}
      <path d="M3 7H15" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
      <path d="M3 12H12" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
      <path d="M3 17H15" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
      {/* eBPF hook intercepting at kernel layer */}
      <path d="M12 8.5V16Q12 20.5 16.5 20.5H21" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}
