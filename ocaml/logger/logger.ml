open Unix
let addr = (ADDR_INET (inet_addr_of_string "127.0.0.1", 9627))
let send fd s =
  let rec loop off =
    if off = String.length s then ()
    else let n = write fd s off (String.length s - off) in loop (off + n)
  in loop 0
let main () =
  let fd = socket PF_INET SOCK_STREAM 0 in
  connect fd addr;
  let ic = in_channel_of_descr stdin in
  try
    while true do
      let line = input_line ic in
      let payload = line ^ "\n" in
      send fd payload
    done
  with End_of_file -> close fd
let () = main ()
