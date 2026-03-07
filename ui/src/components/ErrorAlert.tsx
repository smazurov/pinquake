export function ErrorAlert({ message }: Readonly<{ message: string }>) {
  return (
    <div className="rounded bg-red-900/50 px-3 py-2 text-xs text-red-300">
      {message}
    </div>
  );
}
