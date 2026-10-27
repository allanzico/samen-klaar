export const PLATFORM_FEE_BASIS_POINTS = 500;

export function professionalPayment(totalMinor: number, basisPoints = 0) {
  if (!Number.isSafeInteger(totalMinor) || totalMinor < 0 || !Number.isInteger(basisPoints) || basisPoints < 0 || basisPoints > 10000) return null;
  const feeMinor = Math.floor(totalMinor / 10000) * basisPoints + Math.floor(((totalMinor % 10000) * basisPoints + 5000) / 10000);
  return { totalMinor, feeMinor, proceedsMinor: totalMinor - feeMinor };
}
