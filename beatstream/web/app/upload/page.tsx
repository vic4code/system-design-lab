import UploadForm from "@/components/UploadForm";

export default function UploadPage() {
  return (
    <div
      className="min-h-full"
      style={{ background: "linear-gradient(to bottom, #2a2a1a 0%, #121212 40%)" }}
    >
      <div className="px-6 pt-14 pb-6 max-w-lg">
        <h1 className="text-3xl font-bold mb-8">Upload a Track</h1>
        <UploadForm />
      </div>
    </div>
  );
}
