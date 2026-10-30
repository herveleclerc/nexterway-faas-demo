export default async function(context) {
    const query = context.request.query || {};
    const name = query.name || "World";
    return {
        status: 200,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
            message: `Hello ${name} from Fission FaaS from NexterWay!`,
            timestamp: new Date().toISOString()
        }) + "\n"
    };
};
