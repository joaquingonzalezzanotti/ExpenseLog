import { formatInTimeZone } from "date-fns-tz";
import { subDays } from "date-fns";

const TZ = "America/Argentina/Buenos_Aires";

function main() {
  const contextDate = formatInTimeZone(new Date(), TZ, "yyyy-MM-dd'T'HH:mm:ssXXX");
  const yesterday = formatInTimeZone(subDays(new Date(), 1), TZ, "yyyy-MM-dd");

  console.log("--- ExpenseLog AI Parser: quick manual checks ---");
  console.log(`Context Date: ${contextDate}`);
  console.log(`Yesterday: ${yesterday}`);

  console.log("\n1) Text only request:");
  console.log(`curl -X POST http://localhost:3000/api/parse \\
  -H "Content-Type: application/json" \\
  -d '{"text":"Gaste 1600 en super hoy","context_date":"${contextDate}"}'`);

  console.log("\n2) Media only request (image/pdf in base64):");
  console.log(`curl -X POST http://localhost:3000/api/parse \\
  -H "Content-Type: application/json" \\
  -d '{"context_date":"${contextDate}","media":{"mime_type":"image/jpeg","data_base64":"<BASE64>"}}'`);

  console.log("\n3) Hybrid request (text + media):");
  console.log(`curl -X POST http://localhost:3000/api/parse \\
  -H "Content-Type: application/json" \\
  -d '{"text":"Creo que fue ayer","media":{"mime_type":"application/pdf","data_base64":"<BASE64>"}}'`);
}

main();
