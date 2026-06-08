export default function Mark({ size = 24 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 32 32" fill="none"
      stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <rect x="4" y="4" width="24" height="24" rx="5.5" />
      <path d="M10 11 H17 a4.5 4.5 0 0 1 4.5 4.5 V22" />
      <rect x="14.5" y="9" width="5" height="5" rx="1.2" fill="currentColor" stroke="none" />
    </svg>
  )
}
